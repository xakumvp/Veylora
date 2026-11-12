package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"veylora/internal/storage"
	"veylora/internal/validation"
	"veylora/internal/workflow"
)

// ExportResult tells the GUI where the file went and which secrets were removed.
type ExportResult struct {
	Path           string   `json:"path"`
	RemovedSecrets []string `json:"removedSecrets"`
}

// ImportResult is the newly created workflow plus non-fatal warnings.
type ImportResult struct {
	Workflow storage.WorkflowMeta `json:"workflow"`
	Warnings []string             `json:"warnings"`
}

var unsafeFileChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

// ExportWorkflow saves the stored workflow to a JSON file chosen by the user.
// Returns nil when the dialog was cancelled.
func (a *App) ExportWorkflow(id string) (*ExportResult, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	w, err := s.GetWorkflow(a.ctx, id)
	if err != nil {
		return nil, a.userError("экспорт", err)
	}
	file, err := workflow.BuildExport(w.Name, w.Description, a.version, w.Document)
	if err != nil {
		return nil, a.userError("экспорт", err)
	}
	name := strings.TrimSpace(unsafeFileChars.ReplaceAllString(w.Name, "_"))
	if name == "" {
		name = "workflow"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Экспорт Workflow",
		DefaultFilename: name + ".veylora.json",
		Filters:         []runtime.FileFilter{{DisplayName: "Veylora Workflow (*.json)", Pattern: "*.json"}},
	})
	if err != nil {
		return nil, a.userError("экспорт", err)
	}
	if path == "" {
		return nil, nil
	}
	if filepath.Ext(path) == "" {
		path += ".json"
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, a.userError("экспорт", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, a.userError("запись файла", err)
	}
	removed := file.RemovedSecrets
	if removed == nil {
		removed = []string{}
	}
	return &ExportResult{Path: path, RemovedSecrets: removed}, nil
}

// ImportWorkflow reads a JSON file chosen by the user, validates it and adds it to the project.
// Returns nil when the dialog was cancelled.
func (a *App) ImportWorkflow(projectID string) (*ImportResult, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Импорт Workflow",
		Filters: []runtime.FileFilter{{DisplayName: "Veylora Workflow (*.json)", Pattern: "*.json"}},
	})
	if err != nil {
		return nil, a.userError("импорт", err)
	}
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.New("не удалось прочитать файл")
	}
	if info.Size() > workflow.MaxImportBytes {
		return nil, fmt.Errorf("файл слишком большой (максимум %d МБ)", workflow.MaxImportBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("не удалось прочитать файл")
	}
	return a.importData(projectID, data, s)
}

func (a *App) importData(projectID string, data []byte, s *storage.Store) (*ImportResult, error) {
	file, err := workflow.ParseImport(data)
	if err != nil {
		return nil, err
	}
	issues := validation.Structure(file.Workflow, a.reg)
	if validation.HasErrors(issues) {
		return nil, fmt.Errorf("Workflow в файле повреждён: %s", validation.Summary(issues))
	}
	warnings := []string{}
	if len(file.RemovedSecrets) > 0 {
		warnings = append(warnings, fmt.Sprintf("При экспорте были удалены секреты (%d) — заполните их заново: %s",
			len(file.RemovedSecrets), strings.Join(file.RemovedSecrets, "; ")))
	}
	w, err := s.CreateWorkflow(a.ctx, projectID, file.Name, file.Description, file.Workflow)
	if err != nil {
		return nil, a.userError("импорт", err)
	}
	return &ImportResult{Workflow: w.WorkflowMeta, Warnings: warnings}, nil
}
