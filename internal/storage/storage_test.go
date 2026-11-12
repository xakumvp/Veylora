package storage

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"veylora/internal/events"
	"veylora/internal/workflow"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProjectWorkflowLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	p, err := s.CreateProject(ctx, "  Demo  ", "")
	if err != nil || p.Name != "Demo" {
		t.Fatalf("create project: %v %+v", err, p)
	}
	doc := workflow.New()
	doc.Nodes = append(doc.Nodes, workflow.Node{ID: "a", Type: "trigger.manual", Name: "T", Key: "trigger", Position: workflow.Position{X: 10, Y: 20}})
	w, err := s.CreateWorkflow(ctx, p.ID, "Flow", "", doc)
	if err != nil {
		t.Fatal(err)
	}

	doc.Nodes[0].Position = workflow.Position{X: 99, Y: 42}
	if _, err := s.SaveWorkflowDocument(ctx, w.ID, doc); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Document.Nodes[0].Position.X != 99 || got.Document.Nodes[0].Params == nil {
		t.Fatalf("document not persisted: %+v", got.Document.Nodes[0])
	}

	list, _ := s.ListWorkflows(ctx, p.ID)
	if len(list) != 1 {
		t.Fatalf("want 1 workflow, got %d", len(list))
	}

	// Executions + logs cascade with the project.
	run := events.RunEvent{RunID: "run1", WorkflowID: w.ID, Status: events.StatusRunning, StartedAt: 1}
	if err := s.CreateExecution(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLog(ctx, events.Log{RunID: "run1", Level: "INFO", Message: "hi", Data: map[string]any{"a": 1}, Timestamp: 2}); err != nil {
		t.Fatal(err)
	}
	logs, _ := s.ListLogs(ctx, "run1")
	if len(logs) != 1 || logs[0].Data == nil {
		t.Fatalf("logs: %+v", logs)
	}

	if err := s.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetWorkflow(ctx, w.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workflow should be gone, got %v", err)
	}
	if logs, _ := s.ListLogs(ctx, "run1"); len(logs) != 0 {
		t.Fatal("logs should cascade-delete")
	}
}

func TestInterruptedAndPrune(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, _ := s.CreateProject(ctx, "P", "")
	w, _ := s.CreateWorkflow(ctx, p.ID, "W", "", workflow.New())
	for i, id := range []string{"a", "b", "c"} {
		_ = s.CreateExecution(ctx, events.RunEvent{RunID: id, WorkflowID: w.ID, Status: events.StatusRunning, StartedAt: int64(i)})
	}
	if err := s.MarkInterruptedExecutions(ctx); err != nil {
		t.Fatal(err)
	}
	ex, _ := s.ListExecutions(ctx, w.ID, 10)
	for _, e := range ex {
		if e.Status != events.StatusCancelled {
			t.Fatalf("execution %s left as %s", e.ID, e.Status)
		}
	}
	if err := s.PruneExecutions(ctx, w.ID, 2); err != nil {
		t.Fatal(err)
	}
	ex, _ = s.ListExecutions(ctx, w.ID, 10)
	if len(ex) != 2 || ex[0].ID != "c" {
		t.Fatalf("prune kept wrong rows: %+v", ex)
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	st, _ := s.GetSettings(ctx)
	if !reflect.DeepEqual(st, DefaultSettings()) {
		t.Fatalf("defaults: %+v", st)
	}
	saved, err := s.SaveSettings(ctx, Settings{AllowLocalNetwork: true, MaxResponseMB: 1000, KeepExecutions: 10})
	if err != nil || saved.MaxResponseMB != 50 {
		t.Fatalf("clamp failed: %+v %v", saved, err)
	}
	st, _ = s.GetSettings(ctx)
	if !st.AllowLocalNetwork || st.KeepExecutions != 10 {
		t.Fatalf("settings not persisted: %+v", st)
	}
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.CreateProject(context.Background(), "Persist", "")
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	list, _ := s2.ListProjects(context.Background())
	if len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("data lost after reopen: %+v", list)
	}
}
