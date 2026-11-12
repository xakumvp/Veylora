package nodes

import (
	"context"
	"strings"
)

// MaxLogMessage caps the length of a single log message.
const MaxLogMessage = 10_000

// Logger writes a message to the Execution Panel.
type Logger struct{}

func (Logger) Definition() Definition {
	return Definition{
		Type:        TypeLogger,
		Name:        "Logger",
		Category:    "Logging",
		Description: "Выводит сообщение в Execution Panel. Поддерживает переменные {{...}}.",
		Icon:        "scroll-text",
		KeyPrefix:   "log",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "message", Label: "Сообщение", Kind: ParamText, Required: true, Templatable: true, Placeholder: "Статус ответа: {{http.status}}"},
			{Key: "level", Label: "Уровень", Kind: ParamSelect, Required: true, Default: LevelInfo, Options: []Option{
				{Value: LevelInfo, Label: "INFO"},
				{Value: LevelWarning, Label: "WARNING"},
				{Value: LevelError, Label: "ERROR"},
				{Value: LevelDebug, Label: "DEBUG"},
			}},
			{Key: "variables", Label: "Дополнительные переменные", Kind: ParamKeyValue, Templatable: true,
				Description: "Будут выведены вместе с сообщением."},
		},
		OutputDoc: []Field{
			{Name: "message", Description: "Итоговый текст сообщения"},
			{Name: "level", Description: "Уровень"},
			{Name: "variables", Description: "Вычисленные дополнительные переменные"},
		},
	}
}

func (Logger) Validate(map[string]any) []FieldError { return nil }

func (Logger) Execute(_ context.Context, in *Input) (Result, error) {
	msg := in.Vars.ResolveString(Str(in.Node.Params, "message", ""))
	if len(msg) > MaxLogMessage {
		msg = msg[:MaxLogMessage] + "… (обрезано)"
	}
	level := strings.ToUpper(Str(in.Node.Params, "level", LevelInfo))
	switch level {
	case LevelInfo, LevelWarning, LevelError, LevelDebug:
	default:
		level = LevelInfo
	}
	vars := map[string]any{}
	for _, kv := range KeyValues(in.Node.Params, "variables") {
		if kv.Enabled {
			vars[kv.Key] = in.Vars.ResolveValue(kv.Value)
		}
	}
	var data map[string]any
	if len(vars) > 0 {
		data = vars
	}
	in.Log(level, msg, data)
	return Result{Output: map[string]any{"message": msg, "level": level, "variables": vars}}, nil
}
