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

	"github.com/google/uuid"

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

// errNodeTimeout marks a node that exceeded its own timeout.
var errNodeTimeout = errors.New("превышен таймаут Node")

// WorkflowLoader loads saved workflows for Execute Workflow (subflow) nodes.
type WorkflowLoader interface {
	LoadWorkflow(ctx context.Context, id string) (name string, doc workflow.Document, err error)
}

// Engine runs workflows. It is stateless and safe for concurrent use.
type Engine struct {
	Registry *nodes.Registry
	Logger   *slog.Logger
	// Loader enables subflows; nil disables them.
	Loader WorkflowLoader
	// sleep is replaceable in tests.
	sleep func(ctx context.Context, d time.Duration) error
}

// New creates an engine.
func New(reg *nodes.Registry, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{Registry: reg, Logger: logger, sleep: sleepCtx}
}

// Request is a single run.
type Request struct {
	RunID        string
	WorkflowID   string
	WorkflowName string
	Doc          workflow.Document
	Env          nodes.Env
	// TriggerNodeID selects the trigger that starts the run; "" picks the Manual
	// Trigger (or the left-most trigger on the canvas).
	TriggerNodeID string
	Event         nodes.TriggerEvent
	// EnvVars are the environment variables exposed as {{env.NAME}} (allow-listed).
	EnvVars map[string]string
	// CallStack lists the parent workflow IDs of a subflow run (outermost first).
	CallStack []string
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

// PickTrigger returns the trigger node a run should start from.
func (e *Engine) PickTrigger(doc workflow.Document, requested string) (workflow.Node, error) {
	var first, manual *workflow.Node
	for i := range doc.Nodes {
		n := &doc.Nodes[i]
		def, ok := e.Registry.Def(n.Type)
		if !ok || !def.IsTrigger() {
			continue
		}
		if requested != "" && n.ID == requested {
			return *n, nil
		}
		if first == nil || n.Position.X < first.Position.X || (n.Position.X == first.Position.X && n.Position.Y < first.Position.Y) {
			first = n
		}
		if n.Type == nodes.TypeManualTrigger {
			manual = n
		}
	}
	if requested != "" {
		return workflow.Node{}, fmt.Errorf("триггер %q не найден в Workflow", requested)
	}
	if manual != nil {
		return *manual, nil
	}
	if first != nil {
		return *first, nil
	}
	return workflow.Node{}, errors.New("в Workflow нет триггера")
}

type runState struct {
	req      Request
	vars     *execution.Context
	redactor *execution.Redactor
	sink     Sink
}

func (r *runState) log(level, nodeID, nodeName, msg string) {
	r.sink.Log(events.Log{RunID: r.req.RunID, NodeID: nodeID, NodeName: nodeName, Level: level, Message: r.redactor.String(msg), Timestamp: time.Now().UnixMilli()})
}

// Run executes the workflow synchronously and returns the final run event.
// Cancel ctx to stop it; the node in progress is marked CANCELLED.
func (e *Engine) Run(ctx context.Context, req Request, sink Sink) events.RunEvent {
	ev, _ := e.execute(ctx, req, sink)
	return ev
}

// reservedRoots are engine-provided variables, excluded from subflow outputs.
var reservedRoots = map[string]bool{"execution": true, "workflow": true, "variables": true, "env": true}

func outputsOf(st *runState) map[string]any {
	out := map[string]any{}
	if st == nil {
		return out
	}
	for k, v := range st.vars.Snapshot() {
		if !reservedRoots[k] {
			out[k] = v
		}
	}
	return out
}

// execute runs a workflow and also returns the node outputs (used by subflows).
func (e *Engine) execute(ctx context.Context, req Request, sink Sink) (events.RunEvent, map[string]any) {
	started := time.Now()
	var st *runState
	if req.Event.Source == "" {
		req.Event.Source = nodes.TriggerManual
	}
	run := events.RunEvent{RunID: req.RunID, WorkflowID: req.WorkflowID, Status: events.StatusRunning,
		Trigger: req.Event.Source, StartedAt: started.UnixMilli()}

	finish := func(status, errMsg string) (events.RunEvent, map[string]any) {
		end := time.Now()
		run.Status = status
		run.Error = errMsg
		run.FinishedAt = end.UnixMilli()
		run.DurationMs = end.Sub(started).Milliseconds()
		sink.RunFinished(run)
		return run, outputsOf(st)
	}

	doc := req.Doc
	doc.Normalize()
	order, err := e.Prepare(doc)
	var trigger workflow.Node
	if err == nil {
		trigger, err = e.PickTrigger(doc, req.TriggerNodeID)
	}
	run.TriggerID = trigger.ID
	sink.RunStarted(run)
	st = &runState{req: req, vars: execution.NewContext(), redactor: execution.NewRedactor(), sink: sink}
	if err != nil {
		st.log(nodes.LevelError, "", "", err.Error())
		return finish(events.StatusFailed, err.Error())
	}

	// Workflow-level timeout.
	runCtx := ctx
	if doc.Settings.TimeoutSec > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(doc.Settings.TimeoutSec*float64(time.Second)))
		defer cancel()
	}

	e.seedVariables(st, doc, started)
	collectSecrets(doc, st.redactor)

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
	st.log(nodes.LevelInfo, "", "", fmt.Sprintf("Запуск Workflow (%d Nodes) — триггер «%s», %s", len(order), trigger.Name, sourceLabel(req.Event.Source)))

	active := map[string]bool{trigger.ID: true}
	unhandled := false
	firstErr := ""

	stopRemaining := func(from int, status string) {
		for _, rest := range order[from:] {
			sink.NodeUpdated(events.NodeEvent{RunID: req.RunID, NodeID: rest, Status: status})
		}
	}
	interrupted := func(from int) (events.RunEvent, bool) {
		if runCtx.Err() == nil {
			return events.RunEvent{}, false
		}
		stopRemaining(from, events.StatusCancelled)
		if ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			msg := "превышен таймаут Workflow (" + humanDuration(time.Duration(doc.Settings.TimeoutSec*float64(time.Second))) + ")"
			st.log(nodes.LevelError, "", "", "Выполнение прервано: "+msg)
			ev, _ := finish(events.StatusTimeout, msg)
			return ev, true
		}
		st.log(nodes.LevelWarning, "", "", "Выполнение остановлено")
		ev, _ := finish(events.StatusCancelled, "")
		return ev, true
	}

	for i, id := range order {
		node := byID[id]
		if ev, stop := interrupted(i); stop {
			return ev, outputsOf(st)
		}
		if !active[id] {
			sink.NodeUpdated(events.NodeEvent{RunID: req.RunID, NodeID: id, Status: events.StatusSkipped})
			continue
		}

		var event *nodes.TriggerEvent
		if id == trigger.ID {
			ev := req.Event
			event = &ev
		}
		res, ev, nodeErr := e.runNode(runCtx, st, node, event)
		if nodeErr != nil && runCtx.Err() != nil {
			if evRun, stop := interrupted(i + 1); stop {
				return evRun, outputsOf(st)
			}
		}

		activate := func(errorPort bool, branch string) {
			for _, ed := range outgoing[id] {
				isErr := ed.SourceHandle == nodes.ErrorPortID
				if errorPort != isErr {
					continue
				}
				if errorPort || branch == "" || ed.SourceHandle == branch {
					active[ed.Target] = true
				}
			}
		}

		if nodeErr == nil {
			st.vars.Set(node.Key, res.Output)
			if res.Stop != nil {
				stopRemaining(i+1, events.StatusSkipped)
				msg := "Workflow остановлен узлом «" + node.Name + "»"
				if res.Stop.Message != "" {
					msg += ": " + res.Stop.Message
				}
				if res.Stop.Status == events.StatusFailed {
					st.log(nodes.LevelError, node.ID, node.Name, msg)
					return finish(events.StatusFailed, st.redactor.String(res.Stop.Message))
				}
				st.log(nodes.LevelInfo, node.ID, node.Name, msg)
				return finish(events.StatusSuccess, "")
			}
			activate(false, res.Branch)
			continue
		}

		errOut := map[string]any{"error": ev.Error, "node": node.Name}
		switch node.EffectiveSettings().OnError {
		case workflow.OnErrorContinue:
			st.vars.Set(node.Key, mergeOutput(res.Output, errOut))
			// Continue through the main output; nodes with several outputs (Condition) stop here.
			if def, _ := e.Registry.Def(node.Type); countRegular(def) == 1 {
				activate(false, "")
			}
		case workflow.OnErrorErrorOutput:
			st.vars.Set(node.Key, mergeOutput(res.Output, errOut))
			activate(true, "")
		default:
			// The current path stops: downstream nodes are not activated and become SKIPPED.
			unhandled = true
			if firstErr == "" {
				firstErr = node.Name + ": " + ev.Error
			}
			if res.Output != nil {
				st.vars.Set(node.Key, res.Output)
			}
		}
	}

	if unhandled {
		st.log(nodes.LevelError, "", "", "Workflow завершён с ошибкой")
		return finish(events.StatusFailed, firstErr)
	}
	st.log(nodes.LevelInfo, "", "", "Workflow выполнен за "+humanDuration(time.Since(started)))
	return finish(events.StatusSuccess, "")
}

// seedVariables publishes the engine-provided variable roots.
func (e *Engine) seedVariables(st *runState, doc workflow.Document, started time.Time) {
	st.vars.Set("execution", map[string]any{
		"id":        st.req.RunID,
		"startedAt": started.Format(time.RFC3339),
		"trigger":   st.req.Event.Source,
	})
	st.vars.Set("workflow", map[string]any{"id": st.req.WorkflowID, "name": st.req.WorkflowName})
	vars := map[string]any{}
	for _, v := range doc.Variables {
		vars[v.Key] = v.Value
		if execution.IsSensitiveKey(v.Key) {
			st.redactor.Add(v.Value)
		}
	}
	st.vars.Set("variables", vars)
	env := map[string]any{}
	for k, v := range st.req.EnvVars {
		env[k] = v
		if execution.IsSensitiveKey(k) {
			st.redactor.Add(v)
		}
	}
	st.vars.Set("env", env)
}

func (e *Engine) runNode(ctx context.Context, st *runState, node workflow.Node, event *nodes.TriggerEvent) (res nodes.Result, ev events.NodeEvent, err error) {
	start := time.Now()
	ev = events.NodeEvent{RunID: st.req.RunID, NodeID: node.ID, Status: events.StatusRunning, StartedAt: start.UnixMilli()}
	st.sink.NodeUpdated(ev)

	exec, _ := e.Registry.Get(node.Type)
	settings := node.EffectiveSettings()
	in := &nodes.Input{
		RunID:    st.req.RunID,
		Node:     node,
		Vars:     st.vars,
		Redactor: st.redactor,
		Env:      st.req.Env,
		Event:    event,
		Subflow:  e.subflowRunner(st),
		Log: func(level, msg string, data map[string]any) {
			var d any
			if data != nil {
				d = st.redactor.Value(data)
			}
			st.sink.Log(events.Log{RunID: st.req.RunID, NodeID: node.ID, NodeName: node.Name, Level: level, Message: st.redactor.String(msg), Data: d, Timestamp: time.Now().UnixMilli()})
		},
	}

	attempts := 0
	for {
		attempts++
		res, err = e.attempt(ctx, exec, in, node, settings.TimeoutSec)
		if err == nil || ctx.Err() != nil || attempts >= settings.Retry.MaxAttempts {
			break
		}
		delay := retryDelay(settings.Retry, attempts)
		in.Log(nodes.LevelWarning, fmt.Sprintf("Попытка %d из %d не удалась: %s. Повтор через %s",
			attempts, settings.Retry.MaxAttempts, err.Error(), humanDuration(delay)), nil)
		if sleepErr := e.sleep(ctx, delay); sleepErr != nil {
			err = sleepErr
			break
		}
	}

	end := time.Now()
	ev.FinishedAt = end.UnixMilli()
	ev.DurationMs = end.Sub(start).Milliseconds()
	ev.Attempts = attempts
	if res.Output != nil {
		ev.Output = previewOutput(st.redactor.Value(res.Output))
	}

	switch {
	case err == nil:
		ev.Status = events.StatusSuccess
		ev.Branch = res.Branch
		msg := "Выполнен за " + humanDuration(end.Sub(start))
		if attempts > 1 {
			msg += fmt.Sprintf(" (с попытки %d)", attempts)
		}
		if res.Branch != "" {
			msg += " → ветка " + strings.ToUpper(res.Branch)
		}
		in.Log(nodes.LevelInfo, msg, nil)
	case ctx.Err() != nil:
		ev.Status = events.StatusCancelled
		ev.Error = "отменено"
		err = context.Canceled
	default:
		ev.Status = events.StatusFailed
		ev.Error = st.redactor.String(err.Error())
		e.Logger.Warn("node failed", "run", st.req.RunID, "node", node.ID, "type", node.Type, "attempts", attempts,
			"error", st.redactor.String(fmt.Sprintf("%+v", err)))
		switch settings.OnError {
		case workflow.OnErrorContinue:
			ev.Handled = true
			in.Log(nodes.LevelWarning, ev.Error+" — ошибка пропущена (Continue On Error)", nil)
		case workflow.OnErrorErrorOutput:
			ev.Handled = true
			ev.Branch = nodes.ErrorPortID
			in.Log(nodes.LevelWarning, ev.Error+" — передано на выход ERROR", nil)
		default:
			in.Log(nodes.LevelError, ev.Error, nil)
		}
	}
	st.sink.NodeUpdated(ev)
	return res, ev, err
}

// attempt runs the executor once with an optional node timeout and panic protection.
func (e *Engine) attempt(ctx context.Context, exec nodes.Executor, in *nodes.Input, node workflow.Node, timeoutSec float64) (res nodes.Result, err error) {
	nctx := ctx
	if timeoutSec > 0 {
		var cancel context.CancelFunc
		nctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec*float64(time.Second)))
		defer cancel()
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				e.Logger.Error("node panic", "node", node.ID, "type", node.Type, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
				err = fmt.Errorf("внутренняя ошибка Node: %v", r)
			}
		}()
		res, err = exec.Execute(nctx, in)
	}()
	if err != nil && ctx.Err() == nil && nctx.Err() != nil {
		err = fmt.Errorf("%w (%s)", errNodeTimeout, humanDuration(time.Duration(timeoutSec*float64(time.Second))))
	}
	return res, err
}

func retryDelay(r workflow.Retry, attempt int) time.Duration {
	d := time.Duration(r.DelayMs) * time.Millisecond
	if r.Backoff {
		for i := 1; i < attempt && d < time.Hour; i++ {
			d *= 2
		}
	}
	if r.MaxDelayMs > 0 && d > time.Duration(r.MaxDelayMs)*time.Millisecond {
		d = time.Duration(r.MaxDelayMs) * time.Millisecond
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func countRegular(def nodes.Definition) int {
	n := 0
	for _, p := range def.Outputs {
		if p.Role != "error" {
			n++
		}
	}
	return n
}

func mergeOutput(out map[string]any, extra map[string]any) map[string]any {
	m := map[string]any{}
	for k, v := range out {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func sourceLabel(s string) string {
	switch s {
	case nodes.TriggerManual:
		return "запуск вручную"
	case "schedule":
		return "запуск по расписанию"
	case nodes.TriggerFile:
		return "событие файловой системы"
	case nodes.TriggerStartup:
		return "запуск приложения"
	case SourceSubflow:
		return "вызов из другого Workflow"
	}
	return s
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

// SourceSubflow is the trigger source of a child run started by Execute Workflow.
const SourceSubflow = "subflow"

func (e *Engine) subflowRunner(st *runState) nodes.SubflowRunner {
	if e.Loader == nil {
		return nil
	}
	return func(ctx context.Context, workflowID string, input map[string]any) (nodes.SubflowResult, error) {
		stack := append(append([]string{}, st.req.CallStack...), st.req.WorkflowID)
		if len(stack) > nodes.MaxSubflowDepth {
			return nodes.SubflowResult{}, fmt.Errorf("превышена максимальная глубина вложенности Subflow (%d)", nodes.MaxSubflowDepth)
		}
		for _, id := range stack {
			if id == workflowID {
				return nodes.SubflowResult{}, errors.New("рекурсивный вызов: Workflow не может вызывать сам себя (в том числе через другие Workflow)")
			}
		}
		name, doc, err := e.Loader.LoadWorkflow(ctx, workflowID)
		if err != nil {
			return nodes.SubflowResult{}, errors.New("Workflow для запуска не найден — возможно, он удалён")
		}
		child := Request{
			RunID:        st.req.RunID + "/" + uuid.NewString()[:8],
			WorkflowID:   workflowID,
			WorkflowName: name,
			Doc:          doc,
			Env:          st.req.Env,
			EnvVars:      st.req.EnvVars,
			Event:        nodes.TriggerEvent{Source: SourceSubflow, Data: map[string]any{"input": input, "parentExecutionId": st.req.RunID}},
			CallStack:    stack,
		}
		ev, outputs := e.execute(ctx, child, &childSink{parent: st, name: name})
		return nodes.SubflowResult{ExecutionID: child.RunID, Status: ev.Status, Error: ev.Error, Outputs: outputs, DurationMs: ev.DurationMs}, nil
	}
}

// childSink forwards a subflow's log lines into the parent run.
type childSink struct {
	parent *runState
	name   string
}

func (c *childSink) RunStarted(events.RunEvent)   {}
func (c *childSink) RunFinished(events.RunEvent)  {}
func (c *childSink) NodeUpdated(events.NodeEvent) {}
func (c *childSink) Log(l events.Log) {
	label := "↳ " + c.name
	if l.NodeName != "" {
		label += " / " + l.NodeName
	}
	c.parent.sink.Log(events.Log{RunID: c.parent.req.RunID, NodeName: label, Level: l.Level, Message: c.parent.redactor.String(l.Message), Data: l.Data, Timestamp: l.Timestamp})
}
