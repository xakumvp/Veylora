// Package nodes defines every node type available in Veylora: its ports,
// its parameters (used by the frontend Inspector) and its executor.
package nodes

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"veylora/internal/execution"
	"veylora/internal/workflow"
)

// PortKindFlow is the only port kind in the MVP: control flow + data context.
const PortKindFlow = "flow"

// Port is an input or output connection point of a node.
type Port struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// Role is "error" for the error output that every non-trigger node gets.
	Role string `json:"role,omitempty"`
}

// ErrorPortID is the output used when a node fails and its OnError mode is "error_output".
const ErrorPortID = "error"

// Parameter kinds understood by the Inspector.
const (
	ParamString   = "string"
	ParamText     = "text"
	ParamNumber   = "number"
	ParamSelect   = "select"
	ParamKeyValue = "keyvalue"
	ParamBoolean  = "boolean"
)

// Option is a choice of a select parameter.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ParamSpec describes one configurable parameter of a node.
type ParamSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Options     []Option `json:"options,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	// Templatable means the value may contain {{variables}}.
	Templatable bool `json:"templatable"`
	// OptionalWhen makes the parameter optional for some values of another (select) parameter.
	OptionalWhen *ParamCondition `json:"optionalWhen,omitempty"`
}

// ParamCondition makes a parameter optional for given values of another parameter.
type ParamCondition struct {
	Param string   `json:"param"`
	In    []string `json:"in"`
}

// Field documents one field of a node's output.
type Field struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Definition is the static description of a node type.
type Definition struct {
	Type        string      `json:"type"`
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	Description string      `json:"description"`
	Icon        string      `json:"icon"`
	KeyPrefix   string      `json:"keyPrefix"`
	Inputs      []Port      `json:"inputs"`
	Outputs     []Port      `json:"outputs"`
	Params      []ParamSpec `json:"params"`
	OutputDoc   []Field     `json:"outputDoc"`
	// Trigger is the trigger kind ("manual", "cron", "interval", "schedule", "file", "startup")
	// or empty for regular nodes.
	Trigger string `json:"trigger,omitempty"`
}

// IsTrigger reports whether the definition describes a trigger node.
func (d Definition) IsTrigger() bool { return d.Trigger != "" }

// TriggerEvent describes what started a run.
type TriggerEvent struct {
	// Source is "manual", "schedule", "file" or "startup".
	Source string         `json:"source"`
	Data   map[string]any `json:"data,omitempty"`
}

// StopSignal is returned by the Stop Workflow node to end the run early.
type StopSignal struct {
	Status  string // events.StatusSuccess or events.StatusFailed
	Message string
}

// Env carries application-level limits into executors.
type Env struct {
	AllowLocalNetwork bool
	MaxResponseBytes  int64
	// AllowedDirectories limits filesystem access (file triggers, future file nodes).
	AllowedDirectories []string
}

// LogFunc lets a node write to the Execution Panel.
type LogFunc func(level, message string, data map[string]any)

// Input is everything an executor needs to run one node.
type Input struct {
	RunID    string
	Node     workflow.Node
	Vars     *execution.Context
	Redactor *execution.Redactor
	Log      LogFunc
	Env      Env
	// Event is set for the trigger node that started the run.
	Event *TriggerEvent
}

// Result is what a successfully executed node produces.
type Result struct {
	// Output is published in the variable context under the node's key.
	Output map[string]any
	// Branch is the output port to follow. Empty means "all regular outputs".
	Branch string
	// Stop ends the whole run after this node (Stop Workflow node).
	Stop *StopSignal
}

// FieldError is a parameter validation problem.
type FieldError struct {
	Field   string
	Message string
}

// Executor implements one node type.
type Executor interface {
	Definition() Definition
	// Validate performs node-specific checks beyond the generic ParamSpec rules.
	Validate(params map[string]any) []FieldError
	Execute(ctx context.Context, in *Input) (Result, error)
}

// Log levels used by the Logger node and the engine.
const (
	LevelDebug   = "DEBUG"
	LevelInfo    = "INFO"
	LevelWarning = "WARNING"
	LevelError   = "ERROR"
)

func ptr(f float64) *float64 { return &f }

// ---------- parameter helpers ----------

// Str reads a string parameter, falling back to the spec default.
func Str(params map[string]any, key, def string) string {
	v, ok := params[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

// Num reads a numeric parameter. ok is false if the value is missing or not a number.
func Num(params map[string]any, key string) (float64, bool) {
	v, present := params[key]
	if !present || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, !math.IsNaN(t) && !math.IsInf(t, 0)
	case int:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// Bool reads a boolean parameter.
func Bool(params map[string]any, key string, def bool) bool {
	v, ok := params[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	default:
		return def
	}
}

// KV is one row of a key/value parameter.
type KV struct {
	Key     string
	Value   string
	Enabled bool
}

// KeyValues reads a key/value list parameter: [{"key":..,"value":..,"enabled":..}].
func KeyValues(params map[string]any, key string) []KV {
	raw, ok := params[key].([]any)
	if !ok {
		return nil
	}
	out := make([]KV, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k := strings.TrimSpace(Str(m, "key", ""))
		if k == "" {
			continue
		}
		out = append(out, KV{Key: k, Value: Str(m, "value", ""), Enabled: Bool(m, "enabled", true)})
	}
	return out
}

// HasTemplate reports whether a value contains a {{variable}} reference.
func HasTemplate(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}
