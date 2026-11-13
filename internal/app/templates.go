package app

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"veylora/internal/storage"
	"veylora/internal/validation"
	"veylora/internal/workflow"
)

// Template is a ready-made workflow offered when creating a new one.
type Template struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Nodes       int      `json:"nodes"`
	Triggers    []string `json:"triggers"`
}

func (a *App) loadTemplate(id string) (workflow.ExportFile, error) {
	if a.templates == nil {
		return workflow.ExportFile{}, errors.New("шаблоны недоступны")
	}
	if strings.ContainsAny(id, `/\`) || !strings.HasSuffix(id, ".json") {
		return workflow.ExportFile{}, errors.New("некорректный шаблон")
	}
	data, err := fs.ReadFile(a.templates, path.Join("examples", id))
	if err != nil {
		return workflow.ExportFile{}, errors.New("шаблон не найден")
	}
	return workflow.ParseImport(data)
}

// GetTemplates lists the built-in workflow templates.
func (a *App) GetTemplates() []Template {
	out := []Template{}
	if a.templates == nil {
		return out
	}
	entries, err := fs.ReadDir(a.templates, "examples")
	if err != nil {
		return out
	}
	for _, e := range entries {
		f, err := a.loadTemplate(e.Name())
		if err != nil {
			continue
		}
		t := Template{ID: e.Name(), Name: f.Name, Description: f.Description, Nodes: len(f.Workflow.Nodes), Triggers: []string{}}
		for _, n := range f.Workflow.Nodes {
			if def, ok := a.reg.Def(n.Type); ok && def.IsTrigger() {
				t.Triggers = append(t.Triggers, def.Name)
			}
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CreateWorkflowFromTemplate creates a workflow in a project from a built-in template.
func (a *App) CreateWorkflowFromTemplate(projectID, templateID, name string) (storage.WorkflowMeta, error) {
	s, err := a.db()
	if err != nil {
		return storage.WorkflowMeta{}, err
	}
	f, err := a.loadTemplate(templateID)
	if err != nil {
		return storage.WorkflowMeta{}, err
	}
	if issues := validation.Structure(f.Workflow, a.reg); validation.HasErrors(issues) {
		return storage.WorkflowMeta{}, errors.New("шаблон повреждён: " + validation.Summary(issues))
	}
	if strings.TrimSpace(name) == "" {
		name = f.Name
	}
	w, err := s.CreateWorkflow(a.ctx, projectID, name, f.Description, f.Workflow)
	if err != nil {
		return storage.WorkflowMeta{}, a.userError("создание из шаблона", err)
	}
	return w.WorkflowMeta, nil
}
