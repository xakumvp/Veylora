package app

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"veylora/internal/engine"
	"veylora/internal/events"
	"veylora/internal/nodes"
	"veylora/internal/storage"
	"veylora/internal/webhook"
	"veylora/internal/workflow"
)

// maxLogsPerRun protects the database and the GUI from runaway logging.
const maxLogsPerRun = 5000

// RunWorkflow validates and starts the given graph (the editor's current state).
// triggerNodeID selects the trigger to start from ("" = Manual Trigger or the first trigger).
// It returns the run ID; progress arrives through events.
func (a *App) RunWorkflow(workflowID string, doc workflow.Document, triggerNodeID string) (string, error) {
	s, err := a.db()
	if err != nil {
		return "", err
	}
	w, err := s.GetWorkflow(a.ctx, workflowID)
	if err != nil {
		return "", a.userError("запуск", err)
	}
	doc.Normalize()
	id, err := a.startRun(w.WorkflowMeta, doc, triggerNodeID, nodes.TriggerEvent{Source: nodes.TriggerManual})
	if err != nil {
		return "", a.userError("запуск", err)
	}
	return id, nil
}

// startRun builds an engine request from current settings and launches it.
func (a *App) startRun(meta storage.WorkflowMeta, doc workflow.Document, triggerNodeID string, ev nodes.TriggerEvent) (string, error) {
	s, err := a.db()
	if err != nil {
		return "", err
	}
	st, err := s.GetSettings(a.ctx)
	if err != nil {
		st = storage.DefaultSettings()
	}
	a.runs.SetMaxConcurrent(st.MaxConcurrentRuns)
	envVars := map[string]string{}
	for _, name := range st.AllowedEnvVars {
		if v, ok := os.LookupEnv(name); ok {
			envVars[name] = v
		}
	}
	sink := &runSink{app: a, store: s, emitter: a.emitter(), keep: st.KeepExecutions}
	return a.runs.Start(engine.Request{
		WorkflowID:    meta.ID,
		WorkflowName:  meta.Name,
		Doc:           doc,
		TriggerNodeID: triggerNodeID,
		Event:         ev,
		EnvVars:       envVars,
		Env: nodes.Env{
			AllowLocalNetwork:  st.AllowLocalNetwork,
			MaxResponseBytes:   int64(st.MaxResponseMB) << 20,
			AllowedDirectories: st.AllowedDirectories,
		},
	}, sink)
}

// fireTrigger is called by the scheduler when an automatic trigger is due.
// It always runs the latest saved version of the workflow.
func (a *App) fireTrigger(workflowID, nodeID string, ev nodes.TriggerEvent) {
	s, err := a.db()
	if err != nil {
		return
	}
	ctx := context.Background()
	w, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		a.logger.Warn("trigger: workflow not found", "workflow", workflowID, "error", err)
		a.sched.Deactivate(workflowID)
		return
	}
	if !w.Active {
		a.sched.Deactivate(workflowID)
		return
	}
	if err := s.RecordTriggerFired(ctx, workflowID, nodeID, time.Now().UnixMilli()); err != nil {
		a.logger.Warn("record trigger", "error", err)
	}
	if _, err := a.startRun(w.WorkflowMeta, w.Document, nodeID, ev); err != nil {
		level := "warn"
		if !errors.Is(err, engine.ErrAlreadyRunning) && !errors.Is(err, engine.ErrTooManyRuns) {
			level = "error"
		}
		a.logger.Warn("trigger skipped", "workflow", workflowID, "node", nodeID, "source", ev.Source, "level", level, "error", err)
		a.emitter().Emit(events.TriggerSkipped, map[string]any{"workflowId": workflowID, "nodeId": nodeID, "reason": err.Error()})
	}
}

// fireWebhookTrigger handles an incoming HTTP webhook request by starting the target workflow run.
func (a *App) fireWebhookTrigger(workflowID, nodeID string, req webhook.Request) (string, error) {
	s, err := a.db()
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	w, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return "", err
	}
	if !w.Active {
		return "", errors.New("workflow не активен")
	}
	ev := nodes.TriggerEvent{
		Source: nodes.TriggerWebhook,
		Data: map[string]any{
			"endpoint": req.Endpoint,
			"path":     "/webhook/" + req.Endpoint,
			"method":   req.Method,
			"headers":  req.Headers,
			"query":    req.Query,
			"body":     req.Body,
			"rawBody":  req.RawBody,
			"ip":       req.RemoteIP,
		},
	}
	if err := s.RecordTriggerFired(ctx, workflowID, nodeID, time.Now().UnixMilli()); err != nil {
		a.logger.Warn("record webhook trigger", "error", err)
	}
	return a.startRun(w.WorkflowMeta, w.Document, nodeID, ev)
}

// StopRun cancels one run. Returns false if it is not running.
func (a *App) StopRun(runID string) bool {
	return a.runs.Stop(runID)
}

// StopWorkflow cancels the active run of a workflow. Returns false if nothing was running.
func (a *App) StopWorkflow(workflowID string) bool {
	return a.runs.StopWorkflow(workflowID)
}

// GetActiveRuns lists runs in progress.
func (a *App) GetActiveRuns() []engine.ActiveRun {
	return a.runs.Active()
}

// ListExecutions returns the latest runs of a workflow.
func (a *App) ListExecutions(workflowID string) ([]storage.Execution, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	ex, err := s.ListExecutions(a.ctx, workflowID, 200)
	return ex, a.userError("загрузка истории", err)
}

// GetExecutionLogs returns the log of one run.
func (a *App) GetExecutionLogs(executionID string) ([]events.Log, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	l, err := s.ListLogs(a.ctx, executionID)
	return l, a.userError("загрузка логов", err)
}

// ClearExecutions deletes the run history of a workflow.
func (a *App) ClearExecutions(workflowID string) error {
	s, err := a.db()
	if err != nil {
		return err
	}
	if a.runs.IsRunning(workflowID) {
		return errors.New("дождитесь окончания выполнения")
	}
	return a.userError("очистка истории", s.ClearExecutions(a.ctx, workflowID))
}

// runSink persists run progress and forwards it to the GUI.
type runSink struct {
	app     *App
	store   *storage.Store
	emitter events.Emitter
	keep    int

	mu        sync.Mutex
	logs      int
	truncated bool
}

func (r *runSink) RunStarted(ev events.RunEvent) {
	if err := r.store.CreateExecution(context.Background(), ev); err != nil {
		r.app.logger.Error("create execution", "error", err)
	}
	r.emitter.Emit(events.RunStarted, ev)
}

func (r *runSink) NodeUpdated(ev events.NodeEvent) {
	r.emitter.Emit(events.NodeStatus, ev)
}

func (r *runSink) Log(l events.Log) {
	r.mu.Lock()
	r.logs++
	over := r.logs > maxLogsPerRun
	first := over && !r.truncated
	if first {
		r.truncated = true
	}
	r.mu.Unlock()
	if over {
		if !first {
			return
		}
		l = events.Log{RunID: l.RunID, Level: nodes.LevelWarning, Message: "Достигнут лимит логов для одного запуска — дальнейшие сообщения не сохраняются", Timestamp: l.Timestamp}
	}
	if err := r.store.AddLog(context.Background(), l); err != nil {
		r.app.logger.Error("add log", "error", err)
	}
	r.emitter.Emit(events.LogEntry, l)
}

func (r *runSink) RunFinished(ev events.RunEvent) {
	ctx := context.Background()
	if err := r.store.FinishExecution(ctx, ev); err != nil {
		r.app.logger.Error("finish execution", "error", err)
	}
	if err := r.store.PruneExecutions(ctx, ev.WorkflowID, r.keep); err != nil {
		r.app.logger.Warn("prune executions", "error", err)
	}
	r.app.logger.Info("run finished", "run", ev.RunID, "status", ev.Status, "trigger", ev.Trigger, "durationMs", ev.DurationMs)
	r.emitter.Emit(events.RunFinished, ev)
}
