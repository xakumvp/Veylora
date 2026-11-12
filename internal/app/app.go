// Package app wires storage, the engine and the Wails runtime together.
// Every exported method of App is callable from the frontend (Wails bindings).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"veylora/internal/engine"
	"veylora/internal/events"
	"veylora/internal/nodes"
	"veylora/internal/scheduler"
	"veylora/internal/storage"
	"veylora/internal/validation"
	"veylora/internal/workflow"
)

// AppInfo is shown in the About dialog.
type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	DataDir string `json:"dataDir"`
	LogFile string `json:"logFile"`
}

// App is the object bound to the frontend.
type App struct {
	ctx     context.Context
	version string
	dataDir string
	logFile string

	logger  *slog.Logger
	store   *storage.Store
	initErr error
	reg     *nodes.Registry
	engine  *engine.Engine
	runs    *engine.Manager
	sched   *scheduler.Scheduler
	// emit overrides the Wails emitter (tests).
	emit events.Emitter
	// notices are shown to the user once the GUI is ready (e.g. schedules that failed to restore).
	notices []string
}

// SaveResult is returned after saving a workflow.
type SaveResult struct {
	UpdatedAt int64  `json:"updatedAt"`
	Warning   string `json:"warning,omitempty"`
}

// SettingsResult is returned after saving settings.
type SettingsResult struct {
	Settings storage.Settings `json:"settings"`
	Warnings []string         `json:"warnings"`
}

// New creates the application. Nothing touches disk until startup.
func New(version string) *App {
	reg := nodes.NewRegistry()
	return &App{version: version, reg: reg}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dir, err := dataDir()
	if err != nil {
		a.initErr = err
		a.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		return
	}
	a.dataDir = dir
	a.logger, a.logFile = newLogger(dir)
	a.engine = engine.New(a.reg, a.logger)
	a.runs = engine.NewManager(a.engine)

	store, err := storage.Open(filepath.Join(dir, "veylora.db"))
	if err != nil {
		a.initErr = fmt.Errorf("не удалось открыть базу данных: %w", err)
		a.logger.Error("open database", "error", err)
		return
	}
	a.store = store
	if err := store.MarkInterruptedExecutions(ctx); err != nil {
		a.logger.Warn("mark interrupted executions", "error", err)
	}
	if err := a.seed(ctx); err != nil {
		a.logger.Warn("seed example project", "error", err)
	}
	if st, err := store.GetSettings(ctx); err == nil {
		a.runs.SetMaxConcurrent(st.MaxConcurrentRuns)
	}
	a.initScheduler()
	a.notices = append(a.notices, a.restoreSchedules(ctx)...)
	a.sched.FireStartup()
	a.logger.Info("startup", "version", a.version, "dataDir", dir)
}

func (a *App) initScheduler() {
	a.sched = scheduler.New(a.reg, a.fireTrigger, func(wf, node string) int64 {
		at, _ := a.store.TriggerLastFired(context.Background(), wf, node)
		return at
	}, a.logger)
}

// GetStartupNotices returns (once) messages produced while the app was starting.
func (a *App) GetStartupNotices() []string {
	n := a.notices
	a.notices = nil
	if n == nil {
		n = []string{}
	}
	return n
}

func (a *App) shutdown(_ context.Context) {
	if a.sched != nil {
		a.sched.Stop()
	}
	if a.runs != nil {
		a.runs.Shutdown(5 * time.Second)
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			a.logger.Warn("close database", "error", err)
		}
	}
	if a.logger != nil {
		a.logger.Info("shutdown")
	}
}

func dataDir() (string, error) {
	if d := os.Getenv("VEYLORA_DATA_DIR"); d != "" {
		return d, os.MkdirAll(d, 0o700)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("не найден каталог пользователя: %w", err)
	}
	dir := filepath.Join(base, "Veylora")
	return dir, os.MkdirAll(dir, 0o700)
}

func (a *App) db() (*storage.Store, error) {
	if a.store == nil {
		if a.initErr != nil {
			return nil, a.initErr
		}
		return nil, errors.New("база данных ещё не готова")
	}
	return a.store, nil
}

// userError hides internal details (paths, SQL) from the GUI while logging them.
func (a *App) userError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrNotFound) {
		return errors.New("запись не найдена — возможно, она была удалена")
	}
	var ve *engine.ValidationError
	if errors.As(err, &ve) || errors.Is(err, engine.ErrAlreadyRunning) || errors.Is(err, engine.ErrTooManyRuns) {
		return err
	}
	a.logger.Error(op, "error", err)
	return fmt.Errorf("%s: %v", op, err)
}

// ---------- info ----------

// GetAppInfo returns version and data locations.
func (a *App) GetAppInfo() AppInfo {
	return AppInfo{Name: "Veylora", Version: a.version, DataDir: a.dataDir, LogFile: a.logFile}
}

// GetNodeDefinitions lists all node types for the Node Library and Inspector.
func (a *App) GetNodeDefinitions() []nodes.Definition {
	return a.reg.Definitions()
}

// ---------- projects ----------

// ListProjects returns all projects.
func (a *App) ListProjects() ([]storage.Project, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	p, err := s.ListProjects(a.ctx)
	return p, a.userError("загрузка проектов", err)
}

// CreateProject creates a project.
func (a *App) CreateProject(name string) (storage.Project, error) {
	s, err := a.db()
	if err != nil {
		return storage.Project{}, err
	}
	p, err := s.CreateProject(a.ctx, name, "")
	return p, a.userError("создание проекта", err)
}

// RenameProject renames a project.
func (a *App) RenameProject(id, name string) error {
	s, err := a.db()
	if err != nil {
		return err
	}
	return a.userError("переименование проекта", s.RenameProject(a.ctx, id, name))
}

// DeleteProject deletes a project and everything in it.
func (a *App) DeleteProject(id string) error {
	s, err := a.db()
	if err != nil {
		return err
	}
	wfs, err := s.ListWorkflows(a.ctx, id)
	if err != nil {
		return a.userError("удаление проекта", err)
	}
	for _, w := range wfs {
		if a.runs.IsRunning(w.ID) {
			return errors.New("сначала остановите выполняющиеся Workflow этого проекта")
		}
	}
	for _, w := range wfs {
		a.sched.Deactivate(w.ID)
	}
	return a.userError("удаление проекта", s.DeleteProject(a.ctx, id))
}

// ---------- workflows ----------

// ListWorkflows lists workflows of a project.
func (a *App) ListWorkflows(projectID string) ([]storage.WorkflowMeta, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	w, err := s.ListWorkflows(a.ctx, projectID)
	return w, a.userError("загрузка Workflow", err)
}

// CreateWorkflow creates a workflow that already contains a Manual Trigger.
func (a *App) CreateWorkflow(projectID, name string) (storage.Workflow, error) {
	s, err := a.db()
	if err != nil {
		return storage.Workflow{}, err
	}
	w, err := s.CreateWorkflow(a.ctx, projectID, name, "", starterDocument())
	return w, a.userError("создание Workflow", err)
}

// GetWorkflow loads a workflow with its graph.
func (a *App) GetWorkflow(id string) (storage.Workflow, error) {
	s, err := a.db()
	if err != nil {
		return storage.Workflow{}, err
	}
	w, err := s.GetWorkflow(a.ctx, id)
	return w, a.userError("загрузка Workflow", err)
}

// SaveWorkflow stores the graph. Incomplete graphs may be saved; only hard limits are enforced.
// If the workflow is active, its triggers are re-armed with the new version.
func (a *App) SaveWorkflow(id string, doc workflow.Document) (SaveResult, error) {
	s, err := a.db()
	if err != nil {
		return SaveResult{}, err
	}
	doc.Normalize()
	if len(doc.Nodes) > validation.MaxNodes || len(doc.Edges) > validation.MaxEdges {
		return SaveResult{}, fmt.Errorf("Workflow слишком большой (максимум %d Nodes и %d Connections)", validation.MaxNodes, validation.MaxEdges)
	}
	if doc.SchemaVersion != workflow.SchemaVersion {
		return SaveResult{}, fmt.Errorf("неподдерживаемая версия схемы %d", doc.SchemaVersion)
	}
	t, err := s.SaveWorkflowDocument(a.ctx, id, doc)
	if err != nil {
		return SaveResult{}, a.userError("сохранение Workflow", err)
	}
	return SaveResult{UpdatedAt: t, Warning: a.rearm(id)}, nil
}

// RenameWorkflow changes name and description.
func (a *App) RenameWorkflow(id, name, description string) error {
	s, err := a.db()
	if err != nil {
		return err
	}
	return a.userError("переименование Workflow", s.RenameWorkflow(a.ctx, id, name, description))
}

// DeleteWorkflow removes a workflow and its history.
func (a *App) DeleteWorkflow(id string) error {
	s, err := a.db()
	if err != nil {
		return err
	}
	if a.runs.IsRunning(id) {
		return errors.New("сначала остановите выполнение этого Workflow")
	}
	a.sched.Deactivate(id)
	return a.userError("удаление Workflow", s.DeleteWorkflow(a.ctx, id))
}

// ValidateWorkflow returns all issues without running anything.
func (a *App) ValidateWorkflow(doc workflow.Document) []validation.Issue {
	doc.Normalize()
	issues := validation.Validate(doc, a.reg)
	if issues == nil {
		issues = []validation.Issue{}
	}
	return issues
}

// ---------- settings ----------

// GetSettings returns user settings.
func (a *App) GetSettings() (storage.Settings, error) {
	s, err := a.db()
	if err != nil {
		return storage.DefaultSettings(), err
	}
	st, err := s.GetSettings(a.ctx)
	return st, a.userError("загрузка настроек", err)
}

// SaveSettings stores user settings, applies them and re-arms active workflows
// (e.g. file triggers whose directory is no longer allowed are switched off).
func (a *App) SaveSettings(st storage.Settings) (SettingsResult, error) {
	s, err := a.db()
	if err != nil {
		return SettingsResult{Settings: st}, err
	}
	saved, err := s.SaveSettings(a.ctx, st)
	if err != nil {
		return SettingsResult{Settings: st}, a.userError("сохранение настроек", err)
	}
	a.runs.SetMaxConcurrent(saved.MaxConcurrentRuns)
	warnings := a.restoreSchedules(a.ctx)
	if warnings == nil {
		warnings = []string{}
	}
	a.emitter().Emit(events.SchedulerChanged, "")
	return SettingsResult{Settings: saved, Warnings: warnings}, nil
}
