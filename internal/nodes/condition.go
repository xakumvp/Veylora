package nodes

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"veylora/internal/execution"
)

// Condition operators.
const (
	OpEquals      = "equals"
	OpNotEquals   = "not_equals"
	OpGreaterThan = "greater_than"
	OpLessThan    = "less_than"
	OpContains    = "contains"
	OpIsEmpty     = "is_empty"
)

// Condition evaluates an expression and continues through the TRUE or FALSE port.
type Condition struct{}

func (Condition) Definition() Definition {
	return Definition{
		Type:        TypeCondition,
		Name:        "Condition",
		Category:    "Logic",
		Description: "Проверяет условие и направляет выполнение по ветке TRUE или FALSE.",
		Icon:        "git-branch",
		KeyPrefix:   "condition",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs: []Port{
			{ID: "true", Label: "TRUE", Kind: PortKindFlow},
			{ID: "false", Label: "FALSE", Kind: PortKindFlow},
		},
		Params: []ParamSpec{
			{Key: "variable", Label: "Переменная", Kind: ParamString, Required: true, Templatable: true, Placeholder: "{{http.status}}"},
			{Key: "operator", Label: "Оператор", Kind: ParamSelect, Required: true, Default: OpEquals, Options: []Option{
				{Value: OpEquals, Label: "Equals"},
				{Value: OpNotEquals, Label: "Not Equals"},
				{Value: OpGreaterThan, Label: "Greater Than"},
				{Value: OpLessThan, Label: "Less Than"},
				{Value: OpContains, Label: "Contains"},
				{Value: OpIsEmpty, Label: "Is Empty"},
			}},
			{Key: "value", Label: "Значение", Kind: ParamString, Required: true, Templatable: true, Placeholder: "200",
				OptionalWhen: &ParamCondition{Param: "operator", In: []string{OpIsEmpty}}},
		},
		OutputDoc: []Field{
			{Name: "result", Description: "Результат проверки (true / false)"},
			{Name: "left", Description: "Вычисленное значение переменной"},
			{Name: "right", Description: "Значение для сравнения"},
		},
	}
}

func (Condition) Validate(map[string]any) []FieldError { return nil }

func (Condition) Execute(_ context.Context, in *Input) (Result, error) {
	p := in.Node.Params
	op := Str(p, "operator", OpEquals)
	left := in.Vars.ResolveValue(Str(p, "variable", ""))
	right := in.Vars.ResolveValue(Str(p, "value", ""))

	ok, err := Evaluate(left, op, right)
	if err != nil {
		return Result{}, err
	}
	branch := "false"
	if ok {
		branch = "true"
	}
	out := map[string]any{"result": ok, "left": left, "right": right, "operator": op}
	if op == OpIsEmpty {
		delete(out, "right")
	}
	return Result{Output: out, Branch: branch}, nil
}

// Evaluate applies a condition operator to two resolved values.
func Evaluate(left any, op string, right any) (bool, error) {
	switch op {
	case OpIsEmpty:
		return isEmpty(left), nil
	case OpEquals:
		return equal(left, right), nil
	case OpNotEquals:
		return !equal(left, right), nil
	case OpGreaterThan, OpLessThan:
		l, lok := toNumber(left)
		r, rok := toNumber(right)
		if !lok || !rok {
			return false, fmt.Errorf("оператор %q сравнивает только числа: %q и %q",
				opLabel(op), short(execution.Stringify(left)), short(execution.Stringify(right)))
		}
		if op == OpGreaterThan {
			return l > r, nil
		}
		return l < r, nil
	case OpContains:
		return contains(left, right), nil
	default:
		return false, fmt.Errorf("неизвестный оператор %q", op)
	}
}

func opLabel(op string) string {
	switch op {
	case OpGreaterThan:
		return "Greater Than"
	case OpLessThan:
		return "Less Than"
	}
	return op
}

func short(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	case map[string]string:
		return len(t) == 0
	default:
		return false
	}
}

func toNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case bool:
		return 0, false
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func equal(left, right any) bool {
	if ln, ok := toNumber(left); ok {
		if rn, ok := toNumber(right); ok {
			return ln == rn
		}
	}
	if lb, ok := left.(bool); ok {
		return strings.EqualFold(execution.Stringify(right), strconv.FormatBool(lb))
	}
	if rb, ok := right.(bool); ok {
		return strings.EqualFold(execution.Stringify(left), strconv.FormatBool(rb))
	}
	if reflect.DeepEqual(left, right) {
		return true
	}
	return execution.Stringify(left) == execution.Stringify(right)
}

func contains(left, right any) bool {
	switch t := left.(type) {
	case []any:
		for _, item := range t {
			if equal(item, right) {
				return true
			}
		}
		return false
	case map[string]any:
		_, ok := t[execution.Stringify(right)]
		return ok
	default:
		return strings.Contains(execution.Stringify(left), execution.Stringify(right))
	}
}
