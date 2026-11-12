package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"veylora/internal/workflow"
)

// Version sources.
const (
	VersionManual  = "manual"
	VersionAuto    = "auto"
	VersionRestore = "restore"
	VersionImport  = "import"
)

// MaxVersionsPerWorkflow caps history size; automatic snapshots are pruned first.
const MaxVersionsPerWorkflow = 100

// VersionMeta is a version without its document.
type VersionMeta struct {
	ID          int64  `json:"id"`
	WorkflowID  string `json:"workflowId"`
	Number      int    `json:"number"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Nodes       int    `json:"nodes"`
	Edges       int    `json:"edges"`
	CreatedAt   int64  `json:"createdAt"`
}

// Version is a stored snapshot of a workflow document.
type Version struct {
	VersionMeta
	Document workflow.Document `json:"document"`
}

// CreateVersion stores a snapshot unless it is identical to the latest one.
// It returns the version and whether a new row was created.
func (s *Store) CreateVersion(ctx context.Context, workflowID string, doc workflow.Document, source, description string) (VersionMeta, bool, error) {
	doc.Viewport = nil
	data, err := workflow.Marshal(doc)
	if err != nil {
		return VersionMeta{}, false, err
	}
	fp := workflow.Fingerprint(doc)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VersionMeta{}, false, err
	}
	defer tx.Rollback()

	var lastFP string
	var last VersionMeta
	err = tx.QueryRowContext(ctx, `SELECT id, number, description, source, nodes, edges, created_at, fingerprint FROM workflow_versions
		WHERE workflow_id = ? ORDER BY number DESC LIMIT 1`, workflowID).
		Scan(&last.ID, &last.Number, &last.Description, &last.Source, &last.Nodes, &last.Edges, &last.CreatedAt, &lastFP)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return VersionMeta{}, false, err
	}
	if lastFP == fp && source != VersionManual {
		last.WorkflowID = workflowID
		return last, false, nil
	}

	v := VersionMeta{
		WorkflowID:  workflowID,
		Number:      last.Number + 1,
		Description: strings.TrimSpace(description),
		Source:      source,
		Nodes:       len(doc.Nodes),
		Edges:       len(doc.Edges),
		CreatedAt:   now(),
	}
	if r := []rune(v.Description); len(r) > 200 {
		v.Description = string(r[:200])
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO workflow_versions (workflow_id, number, description, source, nodes, edges, fingerprint, workflow_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, workflowID, v.Number, v.Description, v.Source, v.Nodes, v.Edges, fp, string(data), v.CreatedAt)
	if err != nil {
		return VersionMeta{}, false, err
	}
	v.ID, _ = res.LastInsertId()

	// Prune: drop the oldest automatic snapshots first, then the oldest of any kind.
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_versions WHERE workflow_id = ?`, workflowID).Scan(&count); err != nil {
		return VersionMeta{}, false, err
	}
	if extra := count - MaxVersionsPerWorkflow; extra > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_versions WHERE id IN (
			SELECT id FROM workflow_versions WHERE workflow_id = ? AND id != ?
			ORDER BY (source = 'auto') DESC, number ASC LIMIT ?)`, workflowID, v.ID, extra); err != nil {
			return VersionMeta{}, false, err
		}
	}
	return v, true, tx.Commit()
}

// LatestVersionTime returns when the newest version was created (0 if none).
func (s *Store) LatestVersionTime(ctx context.Context, workflowID string) (int64, error) {
	var t sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM workflow_versions WHERE workflow_id = ?`, workflowID).Scan(&t)
	return t.Int64, err
}

// ListVersions returns versions newest first.
func (s *Store) ListVersions(ctx context.Context, workflowID string) ([]VersionMeta, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workflow_id, number, description, source, nodes, edges, created_at
		FROM workflow_versions WHERE workflow_id = ? ORDER BY number DESC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VersionMeta{}
	for rows.Next() {
		var v VersionMeta
		if err := rows.Scan(&v.ID, &v.WorkflowID, &v.Number, &v.Description, &v.Source, &v.Nodes, &v.Edges, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVersion loads one version with its document.
func (s *Store) GetVersion(ctx context.Context, workflowID string, id int64) (Version, error) {
	var v Version
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT id, workflow_id, number, description, source, nodes, edges, created_at, workflow_json
		FROM workflow_versions WHERE id = ? AND workflow_id = ?`, id, workflowID).
		Scan(&v.ID, &v.WorkflowID, &v.Number, &v.Description, &v.Source, &v.Nodes, &v.Edges, &v.CreatedAt, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	doc, err := workflow.Parse([]byte(data))
	if err != nil {
		return Version{}, err
	}
	v.Document = doc
	return v, nil
}
