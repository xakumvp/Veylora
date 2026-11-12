package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"veylora/internal/events"
	"veylora/internal/workflow"
)

// Project groups workflows.
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// WorkflowMeta is a workflow without its graph (for lists).
type WorkflowMeta struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Workflow is a stored workflow with its graph.
type Workflow struct {
	WorkflowMeta
	Document workflow.Document `json:"document"`
}

// Execution is one run of a workflow.
type Execution struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflowId"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	Trigger    string `json:"trigger"`
	TriggerID  string `json:"triggerNodeId"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
	DurationMs int64  `json:"durationMs"`
}

// ---------- projects ----------

// ListProjects returns all projects, oldest first.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, created_at, updated_at FROM projects ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateProject inserts a project.
func (s *Store) CreateProject(ctx context.Context, name, description string) (Project, error) {
	t := now()
	p := Project{ID: newID(), Name: cleanName(name, "Новый проект"), Description: description, CreatedAt: t, UpdatedAt: t}
	_, err := s.db.ExecContext(ctx, `INSERT INTO projects (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, p.CreatedAt, p.UpdatedAt)
	return p, err
}

// RenameProject changes a project's name.
func (s *Store) RenameProject(ctx context.Context, id, name string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE projects SET name = ?, updated_at = ? WHERE id = ?`, cleanName(name, "Проект"), now(), id))
}

// DeleteProject removes a project with all its workflows and executions.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id))
}

// ---------- workflows ----------

// ListWorkflows returns workflows of a project, oldest first.
func (s *Store) ListWorkflows(ctx context.Context, projectID string) ([]WorkflowMeta, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, name, description, active, created_at, updated_at FROM workflows WHERE project_id = ? ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowMeta{}
	for rows.Next() {
		var w WorkflowMeta
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.Name, &w.Description, &w.Active, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// CreateWorkflow inserts a workflow with the given document.
func (s *Store) CreateWorkflow(ctx context.Context, projectID, name, description string, doc workflow.Document) (Workflow, error) {
	data, err := workflow.Marshal(doc)
	if err != nil {
		return Workflow{}, err
	}
	t := now()
	w := Workflow{WorkflowMeta: WorkflowMeta{ID: newID(), ProjectID: projectID, Name: cleanName(name, "Новый Workflow"), Description: description, CreatedAt: t, UpdatedAt: t}, Document: doc}
	_, err = s.db.ExecContext(ctx, `INSERT INTO workflows (id, project_id, name, description, workflow_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.ProjectID, w.Name, w.Description, string(data), w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return Workflow{}, err
	}
	w.Document.Normalize()
	return w, nil
}

// GetWorkflow loads a workflow with its graph.
func (s *Store) GetWorkflow(ctx context.Context, id string) (Workflow, error) {
	var w Workflow
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT id, project_id, name, description, active, workflow_json, created_at, updated_at FROM workflows WHERE id = ?`, id).
		Scan(&w.ID, &w.ProjectID, &w.Name, &w.Description, &w.Active, &data, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Workflow{}, ErrNotFound
	}
	if err != nil {
		return Workflow{}, err
	}
	doc, err := workflow.Parse([]byte(data))
	if err != nil {
		return Workflow{}, err
	}
	w.Document = doc
	return w, nil
}

// SaveWorkflowDocument replaces a workflow's graph.
func (s *Store) SaveWorkflowDocument(ctx context.Context, id string, doc workflow.Document) (int64, error) {
	data, err := workflow.Marshal(doc)
	if err != nil {
		return 0, err
	}
	t := now()
	return t, affected(s.db.ExecContext(ctx, `UPDATE workflows SET workflow_json = ?, updated_at = ? WHERE id = ?`, string(data), t, id))
}

// RenameWorkflow changes a workflow's name and description.
func (s *Store) RenameWorkflow(ctx context.Context, id, name, description string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE workflows SET name = ?, description = ?, updated_at = ? WHERE id = ?`, cleanName(name, "Workflow"), description, now(), id))
}

// SetWorkflowActive turns automatic triggers of a workflow on or off.
func (s *Store) SetWorkflowActive(ctx context.Context, id string, active bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE workflows SET active = ? WHERE id = ?`, active, id))
}

// ListActiveWorkflowIDs returns IDs of workflows whose automatic triggers are enabled.
func (s *Store) ListActiveWorkflowIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM workflows WHERE active = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RecordTriggerFired remembers when a trigger last started a run.
func (s *Store) RecordTriggerFired(ctx context.Context, workflowID, nodeID string, at int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO schedules (workflow_id, node_id, last_fired_at, fire_count) VALUES (?, ?, ?, 1)
		ON CONFLICT(workflow_id, node_id) DO UPDATE SET last_fired_at = excluded.last_fired_at, fire_count = fire_count + 1`, workflowID, nodeID, at)
	return err
}

// TriggerLastFired returns when a trigger last fired (0 if never).
func (s *Store) TriggerLastFired(ctx context.Context, workflowID, nodeID string) (int64, error) {
	var at int64
	err := s.db.QueryRowContext(ctx, `SELECT last_fired_at FROM schedules WHERE workflow_id = ? AND node_id = ?`, workflowID, nodeID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return at, err
}

// ResetTriggerState forgets fire history of a workflow (e.g. after its schedule was edited).
func (s *Store) ResetTriggerState(ctx context.Context, workflowID, nodeID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE workflow_id = ? AND node_id = ?`, workflowID, nodeID)
	return err
}

// DeleteWorkflow removes a workflow and its executions.
func (s *Store) DeleteWorkflow(ctx context.Context, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM workflows WHERE id = ?`, id))
}

// ---------- executions ----------

// CreateExecution records the start of a run.
func (s *Store) CreateExecution(ctx context.Context, ev events.RunEvent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO executions (id, workflow_id, status, trigger, trigger_node_id, started_at) VALUES (?, ?, ?, ?, ?, ?)`,
		ev.RunID, ev.WorkflowID, ev.Status, ev.Trigger, ev.TriggerID, ev.StartedAt)
	return err
}

// FinishExecution records the end of a run.
func (s *Store) FinishExecution(ctx context.Context, ev events.RunEvent) error {
	_, err := s.db.ExecContext(ctx, `UPDATE executions SET status = ?, error = ?, finished_at = ?, duration_ms = ? WHERE id = ?`,
		ev.Status, ev.Error, ev.FinishedAt, ev.DurationMs, ev.RunID)
	return err
}

// MarkInterruptedExecutions closes runs left "RUNNING" by a crash or forced exit.
func (s *Store) MarkInterruptedExecutions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE executions SET status = ?, error = 'выполнение прервано закрытием приложения', finished_at = COALESCE(finished_at, started_at) WHERE status IN (?, ?)`,
		events.StatusCancelled, events.StatusRunning, events.StatusWaiting)
	return err
}

// ListExecutions returns the latest runs of a workflow, newest first.
func (s *Store) ListExecutions(ctx context.Context, workflowID string, limit int) ([]Execution, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workflow_id, status, error, trigger, trigger_node_id, started_at, COALESCE(finished_at, 0), duration_ms FROM executions WHERE workflow_id = ? ORDER BY started_at DESC LIMIT ?`, workflowID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Execution{}
	for rows.Next() {
		var e Execution
		if err := rows.Scan(&e.ID, &e.WorkflowID, &e.Status, &e.Error, &e.Trigger, &e.TriggerID, &e.StartedAt, &e.FinishedAt, &e.DurationMs); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneExecutions keeps only the newest `keep` runs of a workflow.
func (s *Store) PruneExecutions(ctx context.Context, workflowID string, keep int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM executions WHERE workflow_id = ? AND id NOT IN (
		SELECT id FROM executions WHERE workflow_id = ? ORDER BY started_at DESC LIMIT ?)`, workflowID, workflowID, keep)
	return err
}

// ClearExecutions deletes all runs (and logs) of a workflow.
func (s *Store) ClearExecutions(ctx context.Context, workflowID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM executions WHERE workflow_id = ?`, workflowID)
	return err
}

// AddLog appends a log line to an execution.
func (s *Store) AddLog(ctx context.Context, l events.Log) error {
	data := ""
	if l.Data != nil {
		if b, err := json.Marshal(l.Data); err == nil {
			data = string(b)
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO execution_logs (execution_id, node_id, node_name, level, message, data_json, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.RunID, l.NodeID, l.NodeName, l.Level, l.Message, data, l.Timestamp)
	return err
}

// ListLogs returns all log lines of an execution in order.
func (s *Store) ListLogs(ctx context.Context, executionID string) ([]events.Log, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, execution_id, node_id, node_name, level, message, data_json, timestamp FROM execution_logs WHERE execution_id = ? ORDER BY id LIMIT 10000`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []events.Log{}
	for rows.Next() {
		var l events.Log
		var data string
		if err := rows.Scan(&l.ID, &l.RunID, &l.NodeID, &l.NodeName, &l.Level, &l.Message, &data, &l.Timestamp); err != nil {
			return nil, err
		}
		if data != "" {
			var v any
			if json.Unmarshal([]byte(data), &v) == nil {
				l.Data = v
			}
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---------- settings ----------

// Settings are user preferences.
type Settings struct {
	AllowLocalNetwork  bool     `json:"allowLocalNetwork"`
	MaxResponseMB      int      `json:"maxResponseMb"`
	KeepExecutions     int      `json:"keepExecutions"`
	MaxConcurrentRuns  int      `json:"maxConcurrentRuns"`
	AllowedDirectories []string `json:"allowedDirectories"`
	AllowedEnvVars     []string `json:"allowedEnvVars"`
}

// DefaultSettings returns the defaults used on first launch.
func DefaultSettings() Settings {
	return Settings{AllowLocalNetwork: false, MaxResponseMB: 5, KeepExecutions: 50, MaxConcurrentRuns: 4,
		AllowedDirectories: []string{}, AllowedEnvVars: []string{}}
}

func cleanList(in []string, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// Clamp keeps settings within safe bounds.
func (st Settings) Clamp() Settings {
	if st.MaxResponseMB < 1 {
		st.MaxResponseMB = 1
	}
	if st.MaxResponseMB > 50 {
		st.MaxResponseMB = 50
	}
	if st.KeepExecutions < 1 {
		st.KeepExecutions = 1
	}
	if st.KeepExecutions > 500 {
		st.KeepExecutions = 500
	}
	if st.MaxConcurrentRuns < 1 {
		st.MaxConcurrentRuns = 1
	}
	if st.MaxConcurrentRuns > 16 {
		st.MaxConcurrentRuns = 16
	}
	st.AllowedDirectories = cleanList(st.AllowedDirectories, 50)
	st.AllowedEnvVars = cleanList(st.AllowedEnvVars, 100)
	return st
}

// GetSettings loads settings, filling defaults for missing keys.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := DefaultSettings()
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		switch k {
		case "allowLocalNetwork":
			st.AllowLocalNetwork = v == "true"
		case "maxResponseMb":
			if n, err := strconv.Atoi(v); err == nil {
				st.MaxResponseMB = n
			}
		case "keepExecutions":
			if n, err := strconv.Atoi(v); err == nil {
				st.KeepExecutions = n
			}
		case "maxConcurrentRuns":
			if n, err := strconv.Atoi(v); err == nil {
				st.MaxConcurrentRuns = n
			}
		case "allowedDirectories":
			_ = json.Unmarshal([]byte(v), &st.AllowedDirectories)
		case "allowedEnvVars":
			_ = json.Unmarshal([]byte(v), &st.AllowedEnvVars)
		}
	}
	return st.Clamp(), rows.Err()
}

// SaveSettings stores all settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) (Settings, error) {
	st = st.Clamp()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return st, err
	}
	defer tx.Rollback()
	dirs, _ := json.Marshal(st.AllowedDirectories)
	envs, _ := json.Marshal(st.AllowedEnvVars)
	for k, v := range map[string]string{
		"allowLocalNetwork":  strconv.FormatBool(st.AllowLocalNetwork),
		"maxResponseMb":      strconv.Itoa(st.MaxResponseMB),
		"keepExecutions":     strconv.Itoa(st.KeepExecutions),
		"maxConcurrentRuns":  strconv.Itoa(st.MaxConcurrentRuns),
		"allowedDirectories": string(dirs),
		"allowedEnvVars":     string(envs),
	} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return st, err
		}
	}
	return st, tx.Commit()
}
