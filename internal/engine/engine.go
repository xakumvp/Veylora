// Package engine executes workflow graphs: it validates, orders and runs nodes,
// passes data between them and reports progress through a Sink.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"veylora/internal/events"
	"veylora/internal/execution"
	"veylora/internal/nodes"
	"veylora/internal/validation"
	"veylora/internal/workflow"
)

// maxEventOutputBytes caps the node output sent to the GUI; the full value
// still lives in the variable context for downstream nodes.
const maxEventOutputBytes = 256 << 10

// Sink receives everything that happens during a run.
type Sink interface {
	RunStarted(events.RunEvent)
	NodeUpdated(events.NodeEvent)
	Log(events.Log)
	RunFinished(events.RunEvent)
}

// ValidationError is returned when a workflow is not allowed to run.
type ValidationError struct {
	Issues []validation.Issue
}

func (e *ValidationError) Error() string {
	return "Workflow содержит ошибки: " + validation.Summary(e.Issues)
}

// Engine runs workflows. It is stateless and safe for concurrent use.
type Engine struct {
	Registry *nodes.Registry
	Logger   *slog.Logger
}

// New creates an engine.
func New(reg *nodes.Registry, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{Registry: reg, Logger: logger}
}

// Request is a single run.
type Request struct {
	RunID      string
	WorkflowID string
	Doc        workflow.Document
	Env        nodes.Env
}

// Prepare validates the document and returns the execution order.
func (e *Engine) Prepare(doc workflow.Document) ([]string, error) {
	issues := validation.Validate(doc, e.Registry)
	if validation.HasErrors(issues) {
		return nil, &ValidationError{Issues: issues}
	}
	order, cycle := validation.TopoOrder(doc)
	if len(cycle) > 0 {
		return nil, &ValidationError{Issues: []validation.Issue{{Severity: validation.SeverityError, Message: "обнаружен цикл"}}}
	}
	return order, nil
}

// Run executes the workflow synchronously and returns the final run event.
// Cancel ctx to stop it; the node in progress is marked CANCELLED.
func (e *Engine) Run(ctx context.Context, req Request, sink Sink) events.RunEvent {
	started := time.Now()
	run := events.RunEvent{RunID: req.RunID, WorkflowID: req.WorkflowID, Status: events.StatusRunning, StartedAt: started.UnixMilli()}

	finish := func(status, errMsg string) events.RunEvent {
		end := time.Now()
		run.Status = status
		run.Error = errMsg
		run.FinishedAt = end.UnixMilli()
		run.DurationMs = end.Sub(started).Milliseconds()
		sink.RunFinished(run)
		return run
	}

	doc := req.Doc
	doc.Normalize()
	order, err := e.Prepare(doc)
	sink.RunStarted(run)
	if err != nil {
		sink.Log(events.Log{RunID: req.RunID, Level: nodes.LevelError, Message: err.Error(), Timestamp: time.Now().UnixMilli()})
		return finish(events.StatusFailed, err.Error())
	}

	vars := execution.NewContext()
	redactor := execution.NewRedactor()
	collectSecrets(doc, redactor)

	byID := map[string]workflow.Node{}
	outgoing := map[string][]workflow.Edge{}
	for _, n := range doc.Nodes {
		byID[n.ID] = n
	}
	for _, ed := range doc.Edges {
		outgoing[ed.Source] = append(outgoing[ed.Source], ed)
	}

	for _, id := range order {
		sink.NodeUpdated(events.NodeEvent{RunID: req.RunID, NodeID: id, Status: events.StatusWaiting})
	}
	sink.Log(events.Log{RunID: req.RunID, Level: nodes.LevelInfo, Message: fmt.Sprintf("Запуск Workflow (%d Nodes)", len(order)), Timestamp: time.Now().UnixMilli()})

	active := map[string]bool{}
	for _, n := range doc.Nodes {
		if n.Type == nodes.TypeManualTrigger {
			active[n.ID] = true
		}
	}

	failed := false
	firstErr := ""
	for i, id := range order {
		node := byID[id]

		if ctx.Err() != nil {
			for _, rest := range order[i:] {
				sink.NodeUpdated(events.NodeEvent{RunID: req.RunID, NodeID: rest, Status: events.StatusCancelled})
			}
			sink.Log(events.Log{RunID: req.RunID, Level: nodes.LevelWarning, Message: "Выполнение остановлено пользователем", Timestamp: time.Now().UnixMilli()})
			return finish(events.StatusCancelled, "")
		}

		if !active[id] {
			sink.NodeUpdated(events.NodeEvent{RunID: req.RunID, NodeID: id, Status: events.StatusSkipped})
			continue
		}

		res, ev, nodeErr := e.runNode(ctx, req, node, vars, redactor, sink)
		switch {
		case nodeErr == nil:
			vars.Set(node.Key, res.Output)
			for _, ed := range outgoing[id] {
				if res.Branch == "" || ed.SourceHandle == res.Branch {
					active[ed.Target] = true
				}
			}
		case errors.Is(nodeErr, context.Canceled) || ctx.Err() != nil:
			for _, rest := range order[i+1:] {
				sink.NodeUpdated(events.NodeEvent{RunID: req.RunID, NodeID: rest, Status: events.StatusCancelled})
			}
			sink.Log(events.Log{RunID: req.RunID, Level: nodes.LevelWarning, Message: "Выполнение остановлено пользователем", Timestamp: time.Now().UnixMilli()})
			return finish(events.StatusCancelled, "")
		default:
			// The current path stops: downstream nodes are not activated and become SKIPPED.
			failed = true
			if firstErr == "" {
				firstErr = fmt.Sprintf("%s: %s", node.Name, ev.Error)
			}
			if res.Output != nil {
				vars.Set(node.Key, res.Output)
			}
		}
	}

	if failed {
		sink.Log(events.Log{RunID: req.RunID, Level: nodes.LevelError, Message: "Workflow завершён с ошибкой", Timestamp: time.Now().UnixMilli()})
		return finish(events.StatusFailed, firstErr)
	}
	sink.Log(events.Log{RunID: req.RunID, Level: nodes.LevelInfo, Message: "Workflow выполнен за " + humanDuration(time.Since(started)), Timestamp: time.Now().UnixMilli()})
	return finish(events.StatusSuccess, "")
}

func (e *Engine) runNode(ctx context.Context, req Request, node workflow.Node, vars *execution.Context, red *execution.Redactor, sink Sink) (res nodes.Result, ev events.NodeEvent, err error) {
	start := time.Now()
	ev = events.NodeEvent{RunID: req.RunID, NodeID: node.ID, Status: events.StatusRunning, StartedAt: start.UnixMilli()}
	sink.NodeUpdated(ev)

	exec, _ := e.Registry.Get(node.Type)
	in := &nodes.Input{
		RunID:    req.RunID,
		Node:     node,
		Vars:     vars,
		Redactor: red,
		Env:      req.Env,
		Log: func(level, msg string, data map[string]any) {
			var d any
			if data != nil {
				d = red.Value(data)
			}
			sink.Log(events.Log{RunID: req.RunID, NodeID: node.ID, NodeName: node.Name, Level: level, Message: red.String(msg), Data: d, Timestamp: time.Now().UnixMilli()})
		},
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				e.Logger.Error("node panic", "node", node.ID, "type", node.Type, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
				err = fmt.Errorf("внутренняя ошибка Node: %v", r)
			}
		}()
		res, err = exec.Execute(ctx, in)
	}()

	end := time.Now()
	ev.FinishedAt = end.UnixMilli()
	ev.DurationMs = end.Sub(start).Milliseconds()
	if res.Output != nil {
		ev.Output = previewOutput(red.Value(res.Output))
	}

	switch {
	case err == nil:
		ev.Status = events.StatusSuccess
		ev.Branch = res.Branch
		msg := fmt.Sprintf("Выполнен за %d мс", ev.DurationMs)
		if res.Branch != "" {
			msg += fmt.Sprintf(" → ветка %s", stringsUpper(res.Branch))
		}
		in.Log(nodes.LevelInfo, msg, nil)
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		ev.Status = events.StatusCancelled
		ev.Error = "отменено"
		err = context.Canceled
	default:
		ev.Status = events.StatusFailed
		ev.Error = red.String(err.Error())
		// Full (redacted) chain goes to the diagnostic log file.
		e.Logger.Warn("node failed", "run", req.RunID, "node", node.ID, "type", node.Type, "error", red.String(fmt.Sprintf("%+v", err)))
		in.Log(nodes.LevelError, ev.Error, nil)
	}
	sink.NodeUpdated(ev)
	return res, ev, err
}

// collectSecrets registers literal secret values from key/value params (e.g. Authorization headers).
func collectSecrets(doc workflow.Document, red *execution.Redactor) {
	for _, n := range doc.Nodes {
		for key := range n.Params {
			for _, kv := range nodes.KeyValues(n.Params, key) {
				if execution.IsSensitiveKey(kv.Key) && !nodes.HasTemplate(kv.Value) {
					red.Add(kv.Value)
				}
			}
		}
	}
}

func previewOutput(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{"error": "не удалось сериализовать вывод"}
	}
	if len(b) <= maxEventOutputBytes {
		return v
	}
	if m, ok := v.(map[string]any); ok {
		cp := make(map[string]any, len(m))
		for k, val := range m {
			cp[k] = val
		}
		if body, ok := cp["body"]; ok {
			bb, _ := json.Marshal(body)
			cp["body"] = fmt.Sprintf("%s… (обрезано для отображения, %d байт)", truncateUTF8(string(bb), 4096), len(bb))
			if nb, _ := json.Marshal(cp); len(nb) <= maxEventOutputBytes {
				return cp
			}
		}
	}
	return map[string]any{"preview": truncateUTF8(string(b), 4096) + "…", "truncated": true, "size": len(b)}
}

func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}

func stringsUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// humanDuration formats a duration for the Execution Panel: "480 мс", "1,48 с", "2 мин 5 с".
func humanDuration(d time.Duration) string {
	ms := d.Milliseconds()
	switch {
	case ms < 1000:
		return fmt.Sprintf("%d мс", ms)
	case ms < 60_000:
		return strings.Replace(fmt.Sprintf("%.2f с", float64(ms)/1000), ".", ",", 1)
	default:
		return fmt.Sprintf("%d мин %d с", ms/60_000, (ms%60_000)/1000)
	}
}
