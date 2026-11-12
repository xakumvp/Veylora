package nodes

import "sort"

// Node type identifiers. They are stored in workflow files — never rename them.
const (
	TypeManualTrigger = "trigger.manual"
	TypeHTTPRequest   = "http.request"
	TypeCondition     = "logic.condition"
	TypeDelay         = "utility.delay"
	TypeLogger        = "logging.logger"
)

// Categories in the Node Library, in display order.
var Categories = []string{"Triggers", "HTTP", "Logic", "Utilities", "Logging"}

// Registry maps node types to their executors.
type Registry struct {
	byType map[string]Executor
}

// NewRegistry returns a registry with all built-in node types.
func NewRegistry() *Registry {
	r := &Registry{byType: map[string]Executor{}}
	for _, e := range []Executor{
		ManualTrigger{},
		HTTPRequest{},
		Condition{},
		Delay{},
		Logger{},
	} {
		r.byType[e.Definition().Type] = e
	}
	return r
}

// Get returns the executor for a node type.
func (r *Registry) Get(nodeType string) (Executor, bool) {
	e, ok := r.byType[nodeType]
	return e, ok
}

// Definitions lists all node definitions ordered by category, then name.
func (r *Registry) Definitions() []Definition {
	order := map[string]int{}
	for i, c := range Categories {
		order[c] = i
	}
	defs := make([]Definition, 0, len(r.byType))
	for _, e := range r.byType {
		defs = append(defs, e.Definition())
	}
	sort.Slice(defs, func(i, j int) bool {
		if order[defs[i].Category] != order[defs[j].Category] {
			return order[defs[i].Category] < order[defs[j].Category]
		}
		return defs[i].Name < defs[j].Name
	})
	return defs
}
