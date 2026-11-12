package nodes

import (
	"context"
	"time"
)

// ManualTrigger starts a workflow when the user presses Run.
type ManualTrigger struct{}

func (ManualTrigger) Definition() Definition {
	return Definition{
		Type:        TypeManualTrigger,
		Name:        "Manual Trigger",
		Category:    "Triggers",
		Description: "Запускает Workflow вручную по кнопке Run.",
		Icon:        "play",
		KeyPrefix:   "trigger",
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Label: "", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "runName", Label: "Название запуска", Kind: ParamString, Placeholder: "Ручной запуск", Default: "Ручной запуск"},
		},
		OutputDoc: []Field{
			{Name: "startedAt", Description: "Время запуска (RFC 3339)"},
			{Name: "runId", Description: "ID выполнения"},
			{Name: "runName", Description: "Название запуска"},
			{Name: "status", Description: "Статус запуска"},
		},
	}
}

func (ManualTrigger) Validate(map[string]any) []FieldError { return nil }

func (ManualTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	name := Str(in.Node.Params, "runName", "Ручной запуск")
	return Result{Output: map[string]any{
		"startedAt": time.Now().Format(time.RFC3339),
		"runId":     in.RunID,
		"runName":   name,
		"status":    "started",
	}}, nil
}
