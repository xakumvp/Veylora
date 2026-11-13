package app

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"veylora/internal/events"
	"veylora/internal/nodes"
	"veylora/internal/scheduler"
	"veylora/internal/storage"
	"veylora/internal/validation"
	"veylora/internal/webhook"
	"veylora/internal/workflow"
)

// ActivationResult is returned when automatic triggers are switched on or off.
type ActivationResult struct {
	Active   bool                    `json:"active"`
	Triggers []scheduler.TriggerInfo `json:"triggers"`
}

func (a *App) emitter() events.Emitter {
	if a.emit != nil {
		return a.emit
	}
	return events.WailsEmitter{Ctx: a.ctx}
}

// SetWorkflowActive switches the automatic triggers of a workflow on or off.
// Activation validates the saved workflow and every trigger first.
func (a *App) SetWorkflowActive(workflowID string, active bool) (ActivationResult, error) {
	s, err := a.db()
	if err != nil {
		return ActivationResult{}, err
	}
	if !active {
		a.sched.Deactivate(workflowID)
		if a.webhooks != nil {
			a.webhooks.UnregisterWorkflow(workflowID)
		}
		if err := s.SetWorkflowActive(a.ctx, workflowID, false); err != nil {
			return ActivationResult{}, a.userError("отключение", err)
		}
		a.emitter().Emit(events.SchedulerChanged, workflowID)
		return ActivationResult{Active: false, Triggers: []scheduler.TriggerInfo{}}, nil
	}

	w, err := s.GetWorkflow(a.ctx, workflowID)
	if err != nil {
		return ActivationResult{}, a.userError("активация", err)
	}
	if issues := validation.Validate(w.Document, a.reg); validation.HasErrors(issues) {
		return ActivationResult{}, fmt.Errorf("исправьте ошибки перед активацией: %s", validation.Summary(issues))
	}
	st, err := s.GetSettings(a.ctx)
	if err != nil {
		st = storage.DefaultSettings()
	}
	triggers, err := a.sched.Activate(workflowID, w.Document, st.AllowedDirectories)
	if err != nil {
		return ActivationResult{}, err
	}
	if err := s.SetWorkflowActive(a.ctx, workflowID, true); err != nil {
		a.sched.Deactivate(workflowID)
		return ActivationResult{}, a.userError("активация", err)
	}
	a.syncWebhooks(workflowID, w.Document, true)
	a.emitter().Emit(events.SchedulerChanged, workflowID)
	return ActivationResult{Active: true, Triggers: triggers}, nil
}

// GetTriggers lists armed triggers of a workflow ("" = all).
func (a *App) GetTriggers(workflowID string) []scheduler.TriggerInfo {
	if a.sched == nil {
		return []scheduler.TriggerInfo{}
	}
	return a.sched.Triggers(workflowID)
}

// PickDirectory opens a native folder picker. Returns "" if cancelled.
func (a *App) PickDirectory(title string) (string, error) {
	if title == "" {
		title = "Выберите каталог"
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

// restoreSchedules re-arms active workflows after start-up (or a settings change).
// Workflows that can no longer be armed are switched off and reported.
func (a *App) restoreSchedules(ctx context.Context) []string {
	ids, err := a.store.ListActiveWorkflowIDs(ctx)
	if err != nil {
		a.logger.Error("list active workflows", "error", err)
		return nil
	}
	st, err := a.store.GetSettings(ctx)
	if err != nil {
		st = storage.DefaultSettings()
	}
	var problems []string
	for _, id := range ids {
		w, err := a.store.GetWorkflow(ctx, id)
		if err != nil {
			continue
		}
		if _, err := a.sched.Activate(id, w.Document, st.AllowedDirectories); err != nil {
			a.logger.Warn("restore schedule failed", "workflow", id, "error", err)
			a.sched.Deactivate(id)
			if a.webhooks != nil {
				a.webhooks.UnregisterWorkflow(id)
			}
			_ = a.store.SetWorkflowActive(ctx, id, false)
			problems = append(problems, fmt.Sprintf("«%s» отключён: %v", w.Name, err))
		} else {
			a.syncWebhooks(id, w.Document, true)
		}
	}
	return problems
}

// rearm re-activates a workflow after its document changed. Returns a warning if it had to be switched off.
func (a *App) rearm(workflowID string) string {
	w, err := a.store.GetWorkflow(a.ctx, workflowID)
	if err != nil || !w.Active {
		return ""
	}
	st, err := a.store.GetSettings(a.ctx)
	if err != nil {
		st = storage.DefaultSettings()
	}
	disable := func(reason string) string {
		a.sched.Deactivate(workflowID)
		if a.webhooks != nil {
			a.webhooks.UnregisterWorkflow(workflowID)
		}
		_ = a.store.SetWorkflowActive(a.ctx, workflowID, false)
		a.emitter().Emit(events.SchedulerChanged, workflowID)
		return "Автоматизация отключена: " + reason
	}
	if issues := validation.Validate(w.Document, a.reg); validation.HasErrors(issues) {
		return disable(validation.Summary(issues))
	}
	if _, err := a.sched.Activate(workflowID, w.Document, st.AllowedDirectories); err != nil {
		return disable(err.Error())
	}
	a.syncWebhooks(workflowID, w.Document, true)
	a.emitter().Emit(events.SchedulerChanged, workflowID)
	return ""
}

// syncWebhooks registers or unregisters all webhook trigger nodes of a workflow.
func (a *App) syncWebhooks(workflowID string, doc workflow.Document, active bool) {
	if a.webhooks == nil {
		return
	}
	a.webhooks.UnregisterWorkflow(workflowID)
	if !active {
		return
	}
	for _, n := range doc.Nodes {
		if n.Type == nodes.TypeWebhookTrigger {
			ep, _ := n.Params["endpoint"].(string)
			method, _ := n.Params["method"].(string)
			authMode, _ := n.Params["authMode"].(string)
			secret, _ := n.Params["secret"].(string)
			headerName, _ := n.Params["headerName"].(string)
			respCode := 200
			switch c := n.Params["responseCode"].(type) {
			case float64:
				if c > 0 {
					respCode = int(c)
				}
			case int:
				if c > 0 {
					respCode = c
				}
			}
			respBody, _ := n.Params["responseBody"].(string)

			if err := a.webhooks.Register(webhook.Registration{
				WorkflowID:   workflowID,
				NodeID:       n.ID,
				Endpoint:     ep,
				Method:       method,
				AuthMode:     authMode,
				Secret:       secret,
				HeaderName:   headerName,
				ResponseCode: respCode,
				ResponseBody: respBody,
			}); err != nil {
				a.logger.Warn("webhook register failed", "workflow", workflowID, "node", n.ID, "error", err)
			}
		}
	}
}
