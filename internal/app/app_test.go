package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"veylora/internal/engine"
	"veylora/internal/events"
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
	a.engine.Loader = storeLoader{a}
	a.runs = engine.NewManager(a.engine)
	s, err := storage.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a.store = s
	a.emit = events.NopEmitter{}
	a.initScheduler()
	a.initWebhooks(a.ctx)
	t.Cleanup(a.sched.Stop)
	t.Cleanup(func() {
		if a.webhooks != nil && a.webhooks.IsRunning() {
			_ = a.webhooks.Stop(context.Background())
		}
	})
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

func TestActivationLifecycle(t *testing.T) {
	a := testApp(t)
	p, _ := a.CreateProject("Auto")
	w, _ := a.CreateWorkflow(p.ID, "Cron")
	if _, err := a.SetWorkflowActive(w.ID, true); err == nil {
		t.Fatal("manual-only workflow must not activate")
	}

	doc := starterDocument()
	doc.Nodes = append(doc.Nodes, workflow.Node{ID: "cron", Type: "trigger.cron", Name: "Каждый день", Key: "cron",
		Params: map[string]any{"expression": "@daily"}, Position: workflow.Position{X: 80, Y: 400}})
	if _, err := a.SaveWorkflow(w.ID, doc); err != nil {
		t.Fatal(err)
	}
	res, err := a.SetWorkflowActive(w.ID, true)
	if err != nil || !res.Active || len(res.Triggers) != 1 {
		t.Fatalf("activate: %+v %v", res, err)
	}
	got, _ := a.GetWorkflow(w.ID)
	if !got.Active {
		t.Fatal("active flag not persisted")
	}

	// Breaking the cron expression in a save switches automation off with a warning.
	doc.Nodes[1].Params["expression"] = "bad"
	sr, err := a.SaveWorkflow(w.ID, doc)
	if err != nil || sr.Warning == "" {
		t.Fatalf("expected warning, got %+v %v", sr, err)
	}
	got, _ = a.GetWorkflow(w.ID)
	if got.Active || a.sched.IsActive(w.ID) {
		t.Fatal("broken workflow must be deactivated")
	}

	// Restart: active workflows are re-armed from the database.
	doc.Nodes[1].Params["expression"] = "@hourly"
	_, _ = a.SaveWorkflow(w.ID, doc)
	if _, err := a.SetWorkflowActive(w.ID, true); err != nil {
		t.Fatal(err)
	}
	a.sched.Stop()
	a.initScheduler()
	if problems := a.restoreSchedules(context.Background()); len(problems) != 0 || !a.sched.IsActive(w.ID) {
		t.Fatalf("restore failed: %v", problems)
	}
}

func TestVersionHistory(t *testing.T) {
	a := testApp(t)
	p, _ := a.CreateProject("V")
	w, _ := a.CreateWorkflow(p.ID, "Versions")

	doc := starterDocument()
	if _, err := a.SaveWorkflow(w.ID, doc); err != nil { // first save → automatic v1
		t.Fatal(err)
	}
	v2, err := a.CreateVersion(w.ID, "  Только триггер  ")
	if err != nil || v2.Number != 2 || v2.Description != "Только триггер" {
		t.Fatalf("manual version: %+v %v", v2, err)
	}

	doc.Nodes = append(doc.Nodes, workflow.Node{ID: "log", Type: "logging.logger", Name: "Log", Key: "log",
		Params: map[string]any{"message": "x", "level": "INFO"}, Position: workflow.Position{X: 300, Y: 200}})
	if _, err := a.SaveWorkflow(w.ID, doc); err != nil {
		t.Fatal(err)
	}
	diff, err := a.CompareVersion(w.ID, v2.ID)
	if err != nil || len(diff.AddedNodes) != 1 {
		t.Fatalf("diff: %+v %v", diff, err)
	}

	restored, err := a.RestoreVersion(w.ID, v2.ID)
	if err != nil || len(restored.Document.Nodes) != 1 {
		t.Fatalf("restore: %v nodes=%d", err, len(restored.Document.Nodes))
	}
	list, _ := a.ListVersions(w.ID)
	if len(list) != 3 || list[0].Source != "restore" || list[0].Nodes != 2 {
		t.Fatalf("history after restore: %+v", list)
	}
}

func TestTemplates(t *testing.T) {
	a := testApp(t)
	a.templates = os.DirFS(filepath.Join("..", ".."))
	list := a.GetTemplates()
	if len(list) < 3 {
		t.Fatalf("want at least 3 templates, got %d", len(list))
	}
	p, _ := a.CreateProject("T")
	w, err := a.CreateWorkflowFromTemplate(p.ID, list[0].ID, "")
	if err != nil || w.Name != list[0].Name {
		t.Fatalf("create from template: %+v %v", w, err)
	}
	if _, err := a.CreateWorkflowFromTemplate(p.ID, "../go.mod", ""); err == nil {
		t.Fatal("path traversal in template id must be rejected")
	}
}

func TestAppWebhookFlow(t *testing.T) {
	a := testApp(t)
	p, err := a.CreateProject("Webhook Proj")
	if err != nil {
		t.Fatal(err)
	}
	w, err := a.CreateWorkflow(p.ID, "Webhook Flow")
	if err != nil {
		t.Fatal(err)
	}
	doc := workflow.Document{
		SchemaVersion: workflow.SchemaVersion,
		Nodes: []workflow.Node{
			{
				ID:   "hook1",
				Type: "trigger.webhook",
				Name: "Incoming Webhook",
				Key:  "trigger",
				Params: map[string]any{
					"endpoint":     "test-alert",
					"method":       "POST",
					"authMode":     "none",
					"responseCode": 200,
					"responseBody": `{"status":"accepted","run":"{{runId}}"}`,
				},
				Position: workflow.Position{X: 100, Y: 100},
			},
			{
				ID:   "log1",
				Type: "logging.logger",
				Name: "Log Event",
				Key:  "log",
				Params: map[string]any{
					"message": "Received: {{trigger.rawBody}}",
					"level":   "INFO",
				},
				Position: workflow.Position{X: 300, Y: 100},
			},
		},
		Edges: []workflow.Edge{
			{ID: "e1", Source: "hook1", SourceHandle: "out", Target: "log1", TargetHandle: "in"},
		},
	}
	if _, err := a.SaveWorkflow(w.ID, doc); err != nil {
		t.Fatal(err)
	}

	// 1. Activate workflow
	act, err := a.SetWorkflowActive(w.ID, true)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if !act.Active {
		t.Fatal("workflow should be active")
	}

	info := a.GetWebhookInfo()
	if len(info.Endpoints) != 1 || info.Endpoints[0].Endpoint != "test-alert" {
		t.Fatalf("expected endpoint test-alert registered, got %+v", info.Endpoints)
	}

	// 2. Enable webhook server
	st, err := a.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	st.WebhookEnabled = true
	st.WebhookPort = 15678
	st.WebhookHost = "127.0.0.1"
	if _, err := a.SaveSettings(st); err != nil {
		t.Fatal(err)
	}

	info = a.GetWebhookInfo()
	if !info.Running {
		t.Fatalf("webhook server should be running, info: %+v", info)
	}

	// 3. Post to webhook
	reqBody := `{"event":"deploy","env":"prod"}`
	resp, err := http.Post("http://127.0.0.1:15678/webhook/test-alert", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(bodyBytes), `"status":"accepted"`) {
		t.Fatalf("unexpected response body: %s", string(bodyBytes))
	}

	// Wait briefly for engine execution to finish
	time.Sleep(150 * time.Millisecond)

	// Check executions list
	runs, err := a.ListExecutions(w.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("expected 1 execution recorded, got %d (err: %v)", len(runs), err)
	}
	if runs[0].Trigger != "webhook" {
		t.Fatalf("expected trigger source webhook, got %s", runs[0].Trigger)
	}

	// 4. Deactivate workflow
	if _, err := a.SetWorkflowActive(w.ID, false); err != nil {
		t.Fatal(err)
	}
	info = a.GetWebhookInfo()
	if len(info.Endpoints) != 0 {
		t.Fatalf("expected 0 endpoints after deactivation, got %d", len(info.Endpoints))
	}
}
