// Package storage persists projects, workflows, executions and settings in SQLite.
// It uses the pure-Go modernc.org/sqlite driver, so no C compiler is required.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("запись не найдена")

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the database at path. Use ":memory:" for tests.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("создание каталога данных: %w", err)
		}
	}
	dsn := path
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared"
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	dsn += sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("открытие базы данных: %w", err)
	}
	// SQLite allows a single writer; one connection avoids "database is locked" surprises.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

var migrations = []string{
	// v1
	`CREATE TABLE projects (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	);
	CREATE TABLE workflows (
		id            TEXT PRIMARY KEY,
		project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		name          TEXT NOT NULL,
		description   TEXT NOT NULL DEFAULT '',
		workflow_json TEXT NOT NULL,
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	);
	CREATE INDEX idx_workflows_project ON workflows(project_id);
	CREATE TABLE executions (
		id          TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
		status      TEXT NOT NULL,
		error       TEXT NOT NULL DEFAULT '',
		started_at  INTEGER NOT NULL,
		finished_at INTEGER,
		duration_ms INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX idx_executions_workflow ON executions(workflow_id, started_at DESC);
	CREATE TABLE execution_logs (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
		node_id      TEXT NOT NULL DEFAULT '',
		node_name    TEXT NOT NULL DEFAULT '',
		level        TEXT NOT NULL,
		message      TEXT NOT NULL,
		data_json    TEXT NOT NULL DEFAULT '',
		timestamp    INTEGER NOT NULL
	);
	CREATE INDEX idx_logs_execution ON execution_logs(execution_id, id);
	CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,
	// v2: automation — active workflows, trigger source, schedule state
	`ALTER TABLE workflows ADD COLUMN active INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE executions ADD COLUMN trigger TEXT NOT NULL DEFAULT 'manual';
	ALTER TABLE executions ADD COLUMN trigger_node_id TEXT NOT NULL DEFAULT '';
	CREATE INDEX idx_executions_status ON executions(workflow_id, status);
	CREATE TABLE schedules (
		workflow_id   TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
		node_id       TEXT NOT NULL,
		last_fired_at INTEGER NOT NULL DEFAULT 0,
		fire_count    INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workflow_id, node_id)
	);`,
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("чтение версии схемы: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("база данных создана более новой версией Veylora (схема v%d)", version)
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("миграция v%d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() int64 { return time.Now().UnixMilli() }

func newID() string { return uuid.NewString() }

func cleanName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	return name
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
