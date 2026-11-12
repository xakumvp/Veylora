package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"veylora/internal/engine"
	"veylora/internal/storage"
	"veylora/internal/validation"
	"veylora/internal/workflow"
)

func testApp(t *testing.T) *App {
	t.Helper()
	a := New("test")
	a.ctx = context.Background()
	a.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	a.engine = engine.New(a.reg, a.logger)
	a.runs = engine.NewManager(a.engine)
	s, err := storage.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a.store = s
	return a
}

func TestBuiltInDocumentsAreValid(t *testing.T) {
	a := testApp(t)
	if issues := validation.Validate(exampleDocument(), a.reg); len(issues) != 0 {
		t.Fatalf("example workflow has issues: %+v", issues)
	}
	if issues := validation.Validate(starterDocument(), a.reg); validation.HasErrors(issues) {
		t.Fatalf("starter workflow has errors: %+v", issues)
	}
}

func TestSeedCreatesExampleOnce(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	if err := a.seed(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.seed(ctx); err != nil {
		t.Fatal(err)
	}
	projects, _ := a.ListProjects()
	if len(projects) != 1 {
		t.Fatalf("want 1 seeded project, got %d", len(projects))
	}
	wfs, _ := a.ListWorkflows(projects[0].ID)
	if len(wfs) != 1 {
		t.Fatalf("want 1 example workflow, got %d", len(wfs))
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	a := testApp(t)
	p, _ := a.CreateProject("Import target")

	file, err := workflow.BuildExport("Example", "", "test", exampleDocument())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(file)
	res, err := a.importData(p.ID, data, a.store)
	if err != nil {
		t.Fatal(err)
	}
	w, err := a.GetWorkflow(res.Workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Document.Nodes) != len(exampleDocument().Nodes) {
		t.Fatalf("imported %d nodes", len(w.Document.Nodes))
	}
}

func TestImportRejectsBrokenGraph(t *testing.T) {
	a := testApp(t)
	p, _ := a.CreateProject("P")
	doc := exampleDocument()
	doc.Edges = append(doc.Edges, workflow.Edge{ID: "x", Source: "ok", SourceHandle: "out", Target: "http", TargetHandle: "in"})
	file, _ := workflow.BuildExport("Cyclic", "", "test", doc)
	data, _ := json.Marshal(file)
	_, err := a.importData(p.ID, data, a.store)
	if err == nil || !strings.Contains(err.Error(), "цикл") {
		t.Fatalf("cyclic import must fail, got %v", err)
	}
	if wfs, _ := a.ListWorkflows(p.ID); len(wfs) != 0 {
		t.Fatal("broken workflow was saved")
	}
}

func TestSaveRejectsHugeWorkflow(t *testing.T) {
	a := testApp(t)
	p, _ := a.CreateProject("P")
	w, _ := a.CreateWorkflow(p.ID, "W")
	doc := workflow.New()
	for i := 0; i <= validation.MaxNodes; i++ {
		doc.Nodes = append(doc.Nodes, workflow.Node{ID: string(rune('a' + i%26)), Type: "logging.logger"})
	}
	if _, err := a.SaveWorkflow(w.ID, doc); err == nil {
		t.Fatal("oversized workflow saved")
	}
}

func TestExampleFilesImport(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no example files found: %v", err)
	}
	a := testApp(t)
	p, _ := a.CreateProject("Examples")
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := a.importData(p.ID, data, a.store)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
			continue
		}
		w, _ := a.GetWorkflow(res.Workflow.ID)
		for _, i := range validation.Validate(w.Document, a.reg) {
			// Removed secrets are expected to be empty; everything else must be valid.
			if i.Severity == validation.SeverityError {
				t.Errorf("%s: %s", filepath.Base(f), i.Message)
			}
		}
	}
}
