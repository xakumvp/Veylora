package nodes

import "context"

// StopWorkflow ends the run immediately with the chosen status.
type StopWorkflow struct{}

func (StopWorkflow) Definition() Definition {
	return Definition{
		Type:        TypeStopWorkflow,
		Name:        "Stop Workflow",
		Category:    "Logic",
		Description: "Немедленно завершает выполнение Workflow с выбранным статусом.",
		Icon:        "octagon-x",
		KeyPrefix:   "stop",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{},
		Params: []ParamSpec{
			{Key: "status", Label: "Статус", Kind: ParamSelect, Required: true, Default: "success", Options: []Option{
				{Value: "success", Label: "Успех"},
				{Value: "failed", Label: "Ошибка"},
			}},
			{Key: "message", Label: "Сообщение", Kind: ParamString, Templatable: true, Placeholder: "Нечего обрабатывать"},
		},
		OutputDoc: []Field{{Name: "status", Description: "success / failed"}, {Name: "message", Description: "Сообщение"}},
	}
}

func (StopWorkflow) Validate(map[string]any) []FieldError { return nil }

func (StopWorkflow) Execute(_ context.Context, in *Input) (Result, error) {
	status := Str(in.Node.Params, "status", "success")
	msg := in.Vars.ResolveString(Str(in.Node.Params, "message", ""))
	sig := &StopSignal{Status: "SUCCESS", Message: msg}
	if status == "failed" {
		sig.Status = "FAILED"
	}
	return Result{Output: map[string]any{"status": status, "message": msg}, Stop: sig}, nil
}
