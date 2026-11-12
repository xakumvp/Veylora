package app

import (
	"context"
	"sync"

	"veylora/internal/engine"
	"veylora/internal/events"
	"veylora/internal/nodes"
	"veylora/internal/storage"
	"veylora/internal/workflow"
)

// maxLogsPerRun protects the database and the GUI from runaway logging.
const maxLogsPerRun = 5000

// RunWorkflow validates and starts the given graph (the editor's current state).
// It returns the run ID; progress arrives through events.
func (a *App) RunWorkflow(workflowID string, doc workflow.Document) (string, error) {
	s, err := a.db()
	if err != nil {
		return "", err
	}
	if _, err := s.GetWorkflow(a.ctx, workflowID); err != nil {
		return "", a.userError("запуск", err)
	}
	st, err := s.GetSettings(a.ctx)
	if err != nil {
		st = storage.DefaultSettings()
	}
	doc.Normalize()
	sink := &runSink{app: a, store: s, emitter: events.WailsEmitter{Ctx: a.ctx}, keep: st.KeepExecutions}
	id, err := a.runs.Start(engine.Request{
		WorkflowID: workflowID,
		Doc:        doc,
		Env: nodes.Env{
			AllowLocalNetwork: st.AllowLocalNetwork,
			MaxResponseBytes:  int64(st.MaxResponseMB) << 20,
		},
	}, sink)
	if err != nil {
		return "", a.userError("запуск", err)
	}
	return id, nil
}

// StopWorkflow cancels the active run. Returns false if nothing was running.
func (a *App) StopWorkflow() bool {
	return a.runs.Stop()
}

// GetActiveRun returns the ID of the running workflow or "".
func (a *App) GetActiveRun() string {
	return a.runs.ActiveRunID()
}

// ListExecutions returns the latest runs of a workflow.
func (a *App) ListExecutions(workflowID string) ([]storage.Execution, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	ex, err := s.ListExecutions(a.ctx, workflowID, 100)
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
	r.app.logger.Info("run finished", "run", ev.RunID, "status", ev.Status, "durationMs", ev.DurationMs)
	r.emitter.Emit(events.RunFinished, ev)
}
