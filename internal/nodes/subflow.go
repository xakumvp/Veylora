package nodes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaxSubflowDepth limits nesting of Execute Workflow calls.
const MaxSubflowDepth = 5

// DefaultSubflowTimeout applies when the node does not set one.
const DefaultSubflowTimeout = 5 * time.Minute

// ErrSubflowUnavailable is returned when the engine cannot run subflows (e.g. in tests).
var ErrSubflowUnavailable = errors.New("вложенные Workflow недоступны в этом режиме")

// SubflowResult is what a finished child workflow returns.
type SubflowResult struct {
	ExecutionID string
	Status      string
	Error       string
	Outputs     map[string]any
	DurationMs  int64
}

// SubflowRunner executes a child workflow; provided by the engine.
type SubflowRunner func(ctx context.Context, workflowID string, input map[string]any) (SubflowResult, error)

// ParamWorkflow is a parameter that references another workflow by ID.
const ParamWorkflow = "workflow"

// Subflow runs another workflow and returns its results.
type Subflow struct{}

func (Subflow) Definition() Definition {
	return Definition{
		Type:        TypeSubflow,
		Name:        "Execute Workflow",
		Category:    "Logic",
		Description: "Запускает другой Workflow (Subflow) с входными данными и возвращает результаты его шагов.",
		Icon:        "workflow",
		KeyPrefix:   "subflow",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "workflowId", Label: "Workflow", Kind: ParamWorkflow, Required: true,
				Description: "Запускается от своего Manual Trigger. Входные данные доступны в нём как {{trigger.input.имя}}."},
			{Key: "input", Label: "Входные данные", Kind: ParamKeyValue, Templatable: true},
			{Key: "timeout", Label: "Subflow Timeout, сек", Kind: ParamNumber, Required: true, Default: 300, Min: ptr(1), Max: ptr(3600)},
		},
		OutputDoc: []Field{
			{Name: "status", Description: "Статус дочернего запуска"},
			{Name: "outputs", Description: "Результаты шагов: {{subflow.outputs.<ключ>.<поле>}}"},
			{Name: "executionId", Description: "ID дочернего запуска"},
			{Name: "durationMs", Description: "Длительность, мс"},
		},
	}
}

func (Subflow) Validate(p map[string]any) []FieldError { return nil }

func (Subflow) Execute(ctx context.Context, in *Input) (Result, error) {
	if in.Subflow == nil {
		return Result{}, ErrSubflowUnavailable
	}
	id := strings.TrimSpace(Str(in.Node.Params, "workflowId", ""))
	if id == "" {
		return Result{}, fmt.Errorf("не выбран Workflow для запуска")
	}
	input := map[string]any{}
	for _, kv := range KeyValues(in.Node.Params, "input") {
		if kv.Enabled {
			input[kv.Key] = in.Vars.ResolveValue(kv.Value)
		}
	}
	timeout := DefaultSubflowTimeout
	if n, ok := Num(in.Node.Params, "timeout"); ok && n > 0 {
		timeout = time.Duration(n * float64(time.Second))
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := in.Subflow(cctx, id, input)
	if err != nil {
		if ctx.Err() == nil && cctx.Err() != nil {
			return Result{}, fmt.Errorf("превышен Subflow Timeout (%s)", timeout)
		}
		return Result{}, err
	}
	out := map[string]any{
		"status":      res.Status,
		"outputs":     res.Outputs,
		"executionId": res.ExecutionID,
		"durationMs":  float64(res.DurationMs),
	}
	if res.Status != "SUCCESS" {
		if ctx.Err() == nil && cctx.Err() != nil {
			return Result{Output: out}, fmt.Errorf("превышен Subflow Timeout (%s)", timeout)
		}
		msg := res.Error
		if msg == "" {
			msg = strings.ToLower(res.Status)
		}
		return Result{Output: out}, fmt.Errorf("вложенный Workflow завершился со статусом %s: %s", res.Status, msg)
	}
	return Result{Output: out}, nil
}
