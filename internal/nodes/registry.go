package nodes

import "sort"

// Node type identifiers. They are stored in workflow files — never rename them.
const (
	TypeManualTrigger   = "trigger.manual"
	TypeCronTrigger     = "trigger.cron"
	TypeIntervalTrigger = "trigger.interval"
	TypeScheduleTrigger = "trigger.schedule"
	TypeFileTrigger     = "trigger.file"
	TypeStartupTrigger  = "trigger.startup"
	TypeHTTPRequest     = "http.request"
	TypeCondition       = "logic.condition"
	TypeStopWorkflow    = "logic.stop"
	TypeSubflow         = "logic.subflow"
	TypeDelay           = "utility.delay"
	TypeLogger          = "logging.logger"
)

// Categories in the Node Library, in display order.
var Categories = []string{"Triggers", "HTTP", "Logic", "Utilities", "Logging"}

// Registry maps node types to their executors and (decorated) definitions.
type Registry struct {
	byType map[string]Executor
	defs   map[string]Definition
}

// NewRegistry returns a registry with all built-in node types.
func NewRegistry() *Registry { return NewRegistryWith() }

// NewRegistryWith returns the built-in node types plus extra executors
// (used by tests today and by plugins later).
func NewRegistryWith(extra ...Executor) *Registry {
	r := &Registry{byType: map[string]Executor{}, defs: map[string]Definition{}}
	builtins := []Executor{
		ManualTrigger{},
		CronTrigger{},
		IntervalTrigger{},
		ScheduleTrigger{},
		FileTrigger{},
		StartupTrigger{},
		HTTPRequest{},
		Condition{},
		StopWorkflow{},
		Subflow{},
		Delay{},
		Logger{},
	}
	for _, e := range append(builtins, extra...) {
		def := e.Definition()
		// Every regular node can route failures to an error output.
		if !def.IsTrigger() && def.Type != TypeStopWorkflow {
			def.Outputs = append(append([]Port{}, def.Outputs...), Port{ID: ErrorPortID, Label: "ERROR", Kind: PortKindFlow, Role: "error"})
		}
		r.byType[def.Type] = e
		r.defs[def.Type] = def
	}
	return r
}

// Get returns the executor for a node type.
func (r *Registry) Get(nodeType string) (Executor, bool) {
	e, ok := r.byType[nodeType]
	return e, ok
}

// Def returns the full definition (including the error port) of a node type.
func (r *Registry) Def(nodeType string) (Definition, bool) {
	d, ok := r.defs[nodeType]
	return d, ok
}

// Definitions lists all node definitions ordered by category, then name.
func (r *Registry) Definitions() []Definition {
	order := map[string]int{}
	for i, c := range Categories {
		order[c] = i
	}
	defs := make([]Definition, 0, len(r.defs))
	for _, d := range r.defs {
		defs = append(defs, d)
	}
	sort.Slice(defs, func(i, j int) bool {
		if order[defs[i].Category] != order[defs[j].Category] {
			return order[defs[i].Category] < order[defs[j].Category]
		}
		// Manual Trigger first among triggers.
		if (defs[i].Type == TypeManualTrigger) != (defs[j].Type == TypeManualTrigger) {
			return defs[i].Type == TypeManualTrigger
		}
		return defs[i].Name < defs[j].Name
	})
	return defs
}
