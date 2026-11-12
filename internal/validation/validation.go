// Package validation checks that a workflow document is structurally sound
// and that every node is configured correctly before it may run.
package validation

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

// Severity of a validation issue.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Limits protecting the engine and the UI from absurd documents.
const (
	MaxNodes = 500
	MaxEdges = 2000
)

// Issue is a single validation finding.
type Issue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	NodeID   string `json:"nodeId,omitempty"`
	EdgeID   string `json:"edgeId,omitempty"`
	Field    string `json:"field,omitempty"`
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Summary joins error messages into one line for a user-facing error.
func Summary(issues []Issue) string {
	var msgs []string
	for _, i := range issues {
		if i.Severity == SeverityError {
			msgs = append(msgs, i.Message)
		}
	}
	if len(msgs) > 3 {
		return strings.Join(msgs[:3], "; ") + fmt.Sprintf(" и ещё %d", len(msgs)-3)
	}
	return strings.Join(msgs, "; ")
}

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// Validate runs structural and parameter checks — everything required before a run.
func Validate(doc workflow.Document, reg *nodes.Registry) []Issue {
	issues := Structure(doc, reg)
	issues = append(issues, Params(doc, reg)...)
	return issues
}

// Structure checks the graph: nodes, keys, connections, ports, triggers and cycles.
func Structure(doc workflow.Document, reg *nodes.Registry) []Issue {
	var issues []Issue
	errf := func(nodeID, edgeID, format string, args ...any) {
		issues = append(issues, Issue{Severity: SeverityError, NodeID: nodeID, EdgeID: edgeID, Message: fmt.Sprintf(format, args...)})
	}

	if doc.SchemaVersion < 1 || doc.SchemaVersion > workflow.SchemaVersion {
		errf("", "", "неподдерживаемая версия схемы: %d (поддерживается до %d)", doc.SchemaVersion, workflow.SchemaVersion)
		return issues
	}
	if len(doc.Nodes) > MaxNodes {
		errf("", "", "слишком много Nodes: %d (максимум %d)", len(doc.Nodes), MaxNodes)
		return issues
	}
	if len(doc.Edges) > MaxEdges {
		errf("", "", "слишком много Connections: %d (максимум %d)", len(doc.Edges), MaxEdges)
		return issues
	}

	byID := map[string]workflow.Node{}
	keys := map[string]string{}
	triggers := 0
	for _, n := range doc.Nodes {
		if strings.TrimSpace(n.ID) == "" {
			errf("", "", "у Node отсутствует ID")
			continue
		}
		if _, dup := byID[n.ID]; dup {
			errf(n.ID, "", "повторяющийся ID Node: %s", n.ID)
			continue
		}
		byID[n.ID] = n
		if _, ok := reg.Get(n.Type); !ok {
			errf(n.ID, "", "неизвестный тип Node %q", n.Type)
			continue
		}
		if n.Type == nodes.TypeManualTrigger {
			triggers++
		}
		label := nodeLabel(n)
		if strings.TrimSpace(n.Name) == "" {
			errf(n.ID, "", "у Node %s не задано название", label)
		}
		if !keyRe.MatchString(n.Key) {
			issues = append(issues, Issue{Severity: SeverityError, NodeID: n.ID, Field: "key",
				Message: fmt.Sprintf("«%s»: имя переменной должно начинаться с буквы и содержать только латиницу, цифры и _", label)})
		} else if other, dup := keys[n.Key]; dup {
			issues = append(issues, Issue{Severity: SeverityError, NodeID: n.ID, Field: "key",
				Message: fmt.Sprintf("«%s»: имя переменной %q уже используется в «%s»", label, n.Key, other)})
		} else {
			keys[n.Key] = label
		}
	}

	switch {
	case len(doc.Nodes) == 0:
		errf("", "", "Workflow пуст — добавьте Manual Trigger")
	case triggers == 0:
		errf("", "", "в Workflow нет Manual Trigger — выполнению не с чего начаться")
	case triggers > 1:
		errf("", "", "в MVP поддерживается только один Manual Trigger")
	}

	edgeIDs := map[string]bool{}
	seen := map[string]bool{}
	for _, e := range doc.Edges {
		if e.ID == "" || edgeIDs[e.ID] {
			errf("", e.ID, "у Connection отсутствует или повторяется ID")
			continue
		}
		edgeIDs[e.ID] = true
		src, okS := byID[e.Source]
		tgt, okT := byID[e.Target]
		if !okS || !okT {
			errf("", e.ID, "Connection ссылается на несуществующий Node")
			continue
		}
		if e.Source == e.Target {
			errf(e.Source, e.ID, "Node «%s» не может быть соединён сам с собой", nodeLabel(src))
			continue
		}
		srcExec, ok1 := reg.Get(src.Type)
		tgtExec, ok2 := reg.Get(tgt.Type)
		if !ok1 || !ok2 {
			continue // already reported as unknown type
		}
		outPort, ok := findPort(srcExec.Definition().Outputs, e.SourceHandle)
		if !ok {
			errf(src.ID, e.ID, "у «%s» нет выхода %q", nodeLabel(src), e.SourceHandle)
			continue
		}
		inPort, ok := findPort(tgtExec.Definition().Inputs, e.TargetHandle)
		if !ok {
			errf(tgt.ID, e.ID, "у «%s» нет входа %q", nodeLabel(tgt), e.TargetHandle)
			continue
		}
		if outPort.Kind != inPort.Kind {
			errf(tgt.ID, e.ID, "несовместимые порты: %s → %s", outPort.Kind, inPort.Kind)
			continue
		}
		sig := e.Source + "\x00" + e.SourceHandle + "\x00" + e.Target + "\x00" + e.TargetHandle
		if seen[sig] {
			errf(tgt.ID, e.ID, "повторяющееся Connection между «%s» и «%s»", nodeLabel(src), nodeLabel(tgt))
			continue
		}
		seen[sig] = true
	}

	if HasErrors(issues) {
		return issues
	}

	if _, cycle := TopoOrder(doc); len(cycle) > 0 {
		names := make([]string, 0, len(cycle))
		for _, id := range cycle {
			names = append(names, nodeLabel(byID[id]))
		}
		issues = append(issues, Issue{Severity: SeverityError, NodeID: cycle[0],
			Message: "обнаружен цикл: " + strings.Join(names, " → ")})
		return issues
	}

	// Warn about nodes that can never run.
	reachable := Reachable(doc)
	for _, n := range doc.Nodes {
		if !reachable[n.ID] {
			issues = append(issues, Issue{Severity: SeverityWarning, NodeID: n.ID,
				Message: fmt.Sprintf("«%s» не подключён к Manual Trigger и будет пропущен", nodeLabel(n))})
		}
	}
	return issues
}

// Params checks each node's parameters against its spec and node-specific rules.
func Params(doc workflow.Document, reg *nodes.Registry) []Issue {
	var issues []Issue
	for _, n := range doc.Nodes {
		exec, ok := reg.Get(n.Type)
		if !ok {
			continue
		}
		def := exec.Definition()
		label := nodeLabel(n)
		add := func(field, msg string) {
			issues = append(issues, Issue{Severity: SeverityError, NodeID: n.ID, Field: field,
				Message: fmt.Sprintf("«%s» → %s", label, msg)})
		}
		for _, spec := range def.Params {
			if spec.OptionalWhen != nil && contains(spec.OptionalWhen.In, nodes.Str(n.Params, spec.OptionalWhen.Param, defaultString(def, spec.OptionalWhen.Param))) {
				continue
			}
			raw, present := n.Params[spec.Key]
			str := nodes.Str(n.Params, spec.Key, "")
			switch spec.Kind {
			case nodes.ParamString, nodes.ParamText:
				if spec.Required && strings.TrimSpace(str) == "" {
					add(spec.Key, fmt.Sprintf("поле «%s» обязательно", spec.Label))
				}
			case nodes.ParamNumber:
				if !present || raw == nil || str == "" {
					if spec.Required {
						add(spec.Key, fmt.Sprintf("поле «%s» обязательно", spec.Label))
					}
					continue
				}
				v, ok := nodes.Num(n.Params, spec.Key)
				if !ok {
					add(spec.Key, fmt.Sprintf("«%s» должно быть числом", spec.Label))
					continue
				}
				if spec.Min != nil && v < *spec.Min {
					add(spec.Key, fmt.Sprintf("«%s» не может быть меньше %g", spec.Label, *spec.Min))
				}
				if spec.Max != nil && v > *spec.Max {
					add(spec.Key, fmt.Sprintf("«%s» не может быть больше %g", spec.Label, *spec.Max))
				}
			case nodes.ParamSelect:
				val := nodes.Str(n.Params, spec.Key, fmt.Sprint(spec.Default))
				if !optionExists(spec.Options, val) {
					add(spec.Key, fmt.Sprintf("недопустимое значение «%s»: %q", spec.Label, val))
				}
			case nodes.ParamKeyValue:
				if present && raw != nil {
					if _, ok := raw.([]any); !ok {
						add(spec.Key, fmt.Sprintf("«%s» должно быть списком пар ключ/значение", spec.Label))
					}
				}
			case nodes.ParamBoolean:
				if present && raw != nil {
					if _, ok := raw.(bool); !ok {
						add(spec.Key, fmt.Sprintf("«%s» должно быть true/false", spec.Label))
					}
				}
			}
		}
		for _, fe := range exec.Validate(n.Params) {
			add(fe.Field, fe.Message)
		}
	}
	return issues
}

// TopoOrder returns node IDs in a deterministic topological order.
// If the graph has a cycle, cycle holds the IDs along one cycle.
func TopoOrder(doc workflow.Document) (order []string, cycle []string) {
	indeg := map[string]int{}
	adj := map[string][]string{}
	pos := map[string]int{}
	for i, n := range doc.Nodes {
		indeg[n.ID] = 0
		pos[n.ID] = i
	}
	for _, e := range doc.Edges {
		if _, ok := indeg[e.Source]; !ok {
			continue
		}
		if _, ok := indeg[e.Target]; !ok {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
		indeg[e.Target]++
	}
	// Ready queue ordered by canvas position (top-to-bottom, left-to-right) for stable runs.
	less := func(a, b string) bool {
		na, nb := doc.Nodes[pos[a]], doc.Nodes[pos[b]]
		if na.Position.X != nb.Position.X {
			return na.Position.X < nb.Position.X
		}
		if na.Position.Y != nb.Position.Y {
			return na.Position.Y < nb.Position.Y
		}
		return a < b
	}
	var ready []string
	for _, n := range doc.Nodes {
		if indeg[n.ID] == 0 {
			ready = append(ready, n.ID)
		}
	}
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, t := range adj[id] {
			indeg[t]--
			if indeg[t] == 0 {
				ready = append(ready, t)
			}
		}
	}
	if len(order) == len(doc.Nodes) {
		return order, nil
	}
	return order, findCycle(doc, adj, indeg)
}

func findCycle(doc workflow.Document, adj map[string][]string, indeg map[string]int) []string {
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var found []string
	var dfs func(string) bool
	dfs = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		for _, t := range adj[id] {
			if color[t] == grey {
				for i, s := range stack {
					if s == t {
						found = append(append([]string{}, stack[i:]...), t)
						return true
					}
				}
			}
			if color[t] == white && dfs(t) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}
	for _, n := range doc.Nodes {
		if indeg[n.ID] > 0 && color[n.ID] == white && dfs(n.ID) {
			return found
		}
	}
	return []string{}
}

// Reachable returns the set of nodes reachable from any trigger.
func Reachable(doc workflow.Document) map[string]bool {
	adj := map[string][]string{}
	for _, e := range doc.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
	}
	seen := map[string]bool{}
	var queue []string
	for _, n := range doc.Nodes {
		if n.Type == nodes.TypeManualTrigger {
			queue = append(queue, n.ID)
			seen[n.ID] = true
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, t := range adj[id] {
			if !seen[t] {
				seen[t] = true
				queue = append(queue, t)
			}
		}
	}
	return seen
}

func nodeLabel(n workflow.Node) string {
	if strings.TrimSpace(n.Name) != "" {
		return n.Name
	}
	return n.ID
}

func findPort(ports []nodes.Port, id string) (nodes.Port, bool) {
	for _, p := range ports {
		if p.ID == id {
			return p, true
		}
	}
	return nodes.Port{}, false
}

func optionExists(opts []nodes.Option, v string) bool {
	for _, o := range opts {
		if o.Value == v {
			return true
		}
	}
	return false
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func defaultString(def nodes.Definition, key string) string {
	for _, p := range def.Params {
		if p.Key == key && p.Default != nil {
			return fmt.Sprint(p.Default)
		}
	}
	return ""
}
