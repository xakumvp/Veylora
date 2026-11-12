package app

import (
	"context"
	"embed"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

const maxLogFileBytes = 5 << 20

// Run starts the desktop application and blocks until the window is closed.
func Run(assets embed.FS, version string) error {
	a := New(version)
	return wails.Run(&options.App{
		Title:            "Veylora",
		Width:            1440,
		Height:           900,
		MinWidth:         1024,
		MinHeight:        640,
		WindowStartState: options.Maximised,
		BackgroundColour: &options.RGBA{R: 17, G: 18, B: 22, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        a.startup,
		OnShutdown:       a.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "veylora-desktop-7c1e6d2a",
		},
		Bind: []interface{}{a},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
}

// newLogger writes structured JSON logs to <dataDir>/logs/veylora.log,
// rotating the previous file once it grows past maxLogFileBytes.
func newLogger(dir string) (*slog.Logger, string) {
	logDir := filepath.Join(dir, "logs")
	path := filepath.Join(logDir, "veylora.log")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), ""
	}
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogFileBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), ""
	}
	var w io.Writer = f
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})), path
}

// starterDocument is the content of a newly created workflow.
func starterDocument() workflow.Document {
	doc := workflow.New()
	doc.Nodes = append(doc.Nodes, workflow.Node{
		ID: "trigger-1", Type: nodes.TypeManualTrigger, Name: "Manual Trigger", Key: "trigger",
		Params: map[string]any{"runName": "Ручной запуск"}, Position: workflow.Position{X: 80, Y: 200},
	})
	return doc
}

// exampleDocument demonstrates HTTP → Condition → Logger branching.
func exampleDocument() workflow.Document {
	doc := workflow.New()
	doc.Nodes = []workflow.Node{
		{ID: "trigger", Type: nodes.TypeManualTrigger, Name: "Manual Trigger", Key: "trigger",
			Params: map[string]any{"runName": "Пример"}, Position: workflow.Position{X: 40, Y: 200}},
		{ID: "http", Type: nodes.TypeHTTPRequest, Name: "Получить задачу", Key: "http",
			Params: map[string]any{
				"method": "GET", "url": "https://jsonplaceholder.typicode.com/todos/1",
				"headers": []any{}, "query": []any{}, "body": "", "timeout": 15.0, "failOnError": false,
			}, Position: workflow.Position{X: 320, Y: 200}},
		{ID: "check", Type: nodes.TypeCondition, Name: "Статус 200?", Key: "condition",
			Params:   map[string]any{"variable": "{{http.status}}", "operator": "equals", "value": "200"},
			Position: workflow.Position{X: 600, Y: 200}},
		{ID: "delay", Type: nodes.TypeDelay, Name: "Пауза", Key: "delay",
			Params: map[string]any{"duration": 1.0, "unit": "s"}, Position: workflow.Position{X: 880, Y: 100}},
		{ID: "ok", Type: nodes.TypeLogger, Name: "Успех", Key: "log_ok",
			Params: map[string]any{
				"message": "Задача «{{http.body.title}}» получена за {{http.timeMs}} мс", "level": "INFO",
				"variables": []any{map[string]any{"key": "completed", "value": "{{http.body.completed}}", "enabled": true}},
			}, Position: workflow.Position{X: 1160, Y: 100}},
		{ID: "fail", Type: nodes.TypeLogger, Name: "Ошибка", Key: "log_fail",
			Params:   map[string]any{"message": "Сервер ответил статусом {{http.status}}", "level": "ERROR", "variables": []any{}},
			Position: workflow.Position{X: 880, Y: 320}},
	}
	doc.Edges = []workflow.Edge{
		{ID: "e1", Source: "trigger", SourceHandle: "out", Target: "http", TargetHandle: "in"},
		{ID: "e2", Source: "http", SourceHandle: "out", Target: "check", TargetHandle: "in"},
		{ID: "e3", Source: "check", SourceHandle: "true", Target: "delay", TargetHandle: "in"},
		{ID: "e4", Source: "delay", SourceHandle: "out", Target: "ok", TargetHandle: "in"},
		{ID: "e5", Source: "check", SourceHandle: "false", Target: "fail", TargetHandle: "in"},
	}
	return doc
}

// seed creates an example project on the very first launch.
func (a *App) seed(ctx context.Context) error {
	projects, err := a.store.ListProjects(ctx)
	if err != nil || len(projects) > 0 {
		return err
	}
	p, err := a.store.CreateProject(ctx, "Мой первый проект", "")
	if err != nil {
		return err
	}
	_, err = a.store.CreateWorkflow(ctx, p.ID, "Пример: HTTP + Condition", "Демонстрация HTTP Request, Condition, Delay и Logger", exampleDocument())
	return err
}
