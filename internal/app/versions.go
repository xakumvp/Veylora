package app

import (
	"context"
	"fmt"
	"time"

	"veylora/internal/storage"
	"veylora/internal/workflow"
)

// autoVersionEvery is the minimum gap between automatic snapshots taken on save.
const autoVersionEvery = 10 * time.Minute

// CreateVersion stores a named snapshot of the saved workflow.
func (a *App) CreateVersion(workflowID, description string) (storage.VersionMeta, error) {
	s, err := a.db()
	if err != nil {
		return storage.VersionMeta{}, err
	}
	w, err := s.GetWorkflow(a.ctx, workflowID)
	if err != nil {
		return storage.VersionMeta{}, a.userError("создание версии", err)
	}
	v, _, err := s.CreateVersion(a.ctx, workflowID, w.Document, storage.VersionManual, description)
	return v, a.userError("создание версии", err)
}

// ListVersions returns the version history, newest first.
func (a *App) ListVersions(workflowID string) ([]storage.VersionMeta, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	v, err := s.ListVersions(a.ctx, workflowID)
	return v, a.userError("загрузка версий", err)
}

// CompareVersion describes what changed between a version and the current saved workflow.
func (a *App) CompareVersion(workflowID string, versionID int64) (workflow.DiffResult, error) {
	s, err := a.db()
	if err != nil {
		return workflow.DiffResult{}, err
	}
	v, err := s.GetVersion(a.ctx, workflowID, versionID)
	if err != nil {
		return workflow.DiffResult{}, a.userError("сравнение версий", err)
	}
	w, err := s.GetWorkflow(a.ctx, workflowID)
	if err != nil {
		return workflow.DiffResult{}, a.userError("сравнение версий", err)
	}
	return workflow.Diff(v.Document, w.Document), nil
}

// RestoreVersion replaces the workflow with a stored version. The current state is
// snapshotted first, so a restore can itself be undone.
func (a *App) RestoreVersion(workflowID string, versionID int64) (storage.Workflow, error) {
	s, err := a.db()
	if err != nil {
		return storage.Workflow{}, err
	}
	if a.runs.IsRunning(workflowID) {
		// Running executions keep their own copy, but restoring mid-run is confusing.
		return storage.Workflow{}, fmt.Errorf("дождитесь окончания выполнения этого Workflow")
	}
	v, err := s.GetVersion(a.ctx, workflowID, versionID)
	if err != nil {
		return storage.Workflow{}, a.userError("восстановление версии", err)
	}
	w, err := s.GetWorkflow(a.ctx, workflowID)
	if err != nil {
		return storage.Workflow{}, a.userError("восстановление версии", err)
	}
	if _, _, err := s.CreateVersion(a.ctx, workflowID, w.Document, storage.VersionRestore,
		fmt.Sprintf("Перед восстановлением версии %d", v.Number)); err != nil {
		return storage.Workflow{}, a.userError("восстановление версии", err)
	}
	doc := v.Document
	doc.Viewport = w.Document.Viewport
	if _, err := s.SaveWorkflowDocument(a.ctx, workflowID, doc); err != nil {
		return storage.Workflow{}, a.userError("восстановление версии", err)
	}
	if warn := a.rearm(workflowID); warn != "" {
		a.notices = append(a.notices, warn)
	}
	restored, err := s.GetWorkflow(a.ctx, workflowID)
	return restored, a.userError("восстановление версии", err)
}

// ExportVersion saves one historical version to a JSON file. Returns nil if cancelled.
func (a *App) ExportVersion(workflowID string, versionID int64) (*ExportResult, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	w, err := s.GetWorkflow(a.ctx, workflowID)
	if err != nil {
		return nil, a.userError("экспорт версии", err)
	}
	v, err := s.GetVersion(a.ctx, workflowID, versionID)
	if err != nil {
		return nil, a.userError("экспорт версии", err)
	}
	name := fmt.Sprintf("%s (версия %d)", w.Name, v.Number)
	return a.exportDocument(name, w.Description, v.Document)
}

// autoVersion snapshots a saved workflow if the last version is old enough.
func (a *App) autoVersion(ctx context.Context, workflowID string, doc workflow.Document) {
	last, err := a.store.LatestVersionTime(ctx, workflowID)
	if err != nil {
		return
	}
	if last > 0 && time.Since(time.UnixMilli(last)) < autoVersionEvery {
		return
	}
	if _, _, err := a.store.CreateVersion(ctx, workflowID, doc, storage.VersionAuto, ""); err != nil {
		a.logger.Warn("auto version", "workflow", workflowID, "error", err)
	}
}
