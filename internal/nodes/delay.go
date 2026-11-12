package nodes

import (
	"context"
	"fmt"
	"time"
)

// MaxDelay caps a single Delay node.
const MaxDelay = 60 * time.Minute

// Delay pauses the workflow; it is interrupted immediately when the run is stopped.
type Delay struct{}

func (Delay) Definition() Definition {
	return Definition{
		Type:        TypeDelay,
		Name:        "Delay",
		Category:    "Utilities",
		Description: "Приостанавливает выполнение на заданное время. Прерывается кнопкой Stop.",
		Icon:        "timer",
		KeyPrefix:   "delay",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "duration", Label: "Длительность", Kind: ParamNumber, Required: true, Default: 1, Min: ptr(0)},
			{Key: "unit", Label: "Единица", Kind: ParamSelect, Required: true, Default: "s", Options: []Option{
				{Value: "ms", Label: "Milliseconds"},
				{Value: "s", Label: "Seconds"},
				{Value: "min", Label: "Minutes"},
			}},
		},
		OutputDoc: []Field{{Name: "waitedMs", Description: "Сколько миллисекунд длилась пауза"}},
	}
}

func delayDuration(params map[string]any) (time.Duration, error) {
	n, ok := Num(params, "duration")
	if !ok || n < 0 {
		return 0, fmt.Errorf("длительность должна быть неотрицательным числом")
	}
	var unit time.Duration
	switch Str(params, "unit", "s") {
	case "ms":
		unit = time.Millisecond
	case "s":
		unit = time.Second
	case "min":
		unit = time.Minute
	default:
		return 0, fmt.Errorf("неизвестная единица времени")
	}
	if n*float64(unit) > float64(MaxDelay) {
		return 0, fmt.Errorf("максимальная задержка — %d минут", int(MaxDelay.Minutes()))
	}
	return time.Duration(n * float64(unit)), nil
}

func (Delay) Validate(params map[string]any) []FieldError {
	if _, ok := Num(params, "duration"); !ok {
		return nil // generic rules report this
	}
	if _, err := delayDuration(params); err != nil {
		return []FieldError{{Field: "duration", Message: err.Error()}}
	}
	return nil
}

func (Delay) Execute(ctx context.Context, in *Input) (Result, error) {
	d, err := delayDuration(in.Node.Params)
	if err != nil {
		return Result{}, err
	}
	start := time.Now()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-t.C:
	}
	return Result{Output: map[string]any{"waitedMs": time.Since(start).Milliseconds()}}, nil
}
