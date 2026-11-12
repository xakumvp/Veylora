package validation

import (
	"strings"
	"testing"

	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

func node(id, typ, key string, params map[string]any) workflow.Node {
	if params == nil {
		params = map[string]any{}
	}
	return workflow.Node{ID: id, Type: typ, Name: id, Key: key, Params: params}
}

func edge(id, src, sh, tgt string) workflow.Edge {
	return workflow.Edge{ID: id, Source: src, SourceHandle: sh, Target: tgt, TargetHandle: "in"}
}

func validDoc() workflow.Document {
	d := workflow.New()
	d.Nodes = []workflow.Node{
		node("t", nodes.TypeManualTrigger, "trigger", nil),
		node("c", nodes.TypeCondition, "condition", map[string]any{"variable": "{{trigger.status}}", "operator": "equals", "value": "started"}),
		node("l1", nodes.TypeLogger, "log1", map[string]any{"message": "yes", "level": "INFO"}),
		node("l2", nodes.TypeLogger, "log2", map[string]any{"message": "no", "level": "INFO"}),
	}
	d.Edges = []workflow.Edge{edge("e1", "t", "out", "c"), edge("e2", "c", "true", "l1"), edge("e3", "c", "false", "l2")}
	return d
}

func messages(issues []Issue) string {
	var s []string
	for _, i := range issues {
		s = append(s, i.Severity+": "+i.Message)
	}
	return strings.Join(s, "\n")
}

func TestValidWorkflow(t *testing.T) {
	issues := Validate(validDoc(), nodes.NewRegistry())
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got:\n%s", messages(issues))
	}
}

func TestInvalidWorkflows(t *testing.T) {
	reg := nodes.NewRegistry()
	cases := map[string]struct {
		mutate func(*workflow.Document)
		want   string
	}{
		"no trigger":   {func(d *workflow.Document) { d.Nodes = d.Nodes[1:]; d.Edges = nil }, "нет триггера"},
		"unknown type": {func(d *workflow.Document) { d.Nodes[2].Type = "os.exec" }, "неизвестный тип"},
		"missing node": {func(d *workflow.Document) { d.Edges = append(d.Edges, edge("e9", "t", "out", "ghost")) }, "несуществующий Node"},
		"bad handle":   {func(d *workflow.Document) { d.Edges[1].SourceHandle = "maybe" }, "нет выхода"},
		"into trigger": {func(d *workflow.Document) {
			d.Edges = append(d.Edges, workflow.Edge{ID: "e9", Source: "l1", SourceHandle: "out", Target: "t", TargetHandle: "in"})
		}, "нет входа"},
		"self loop": {func(d *workflow.Document) { d.Edges = append(d.Edges, edge("e9", "l1", "out", "l1")) }, "сам с собой"},
		"cycle": {func(d *workflow.Document) {
			d.Edges = append(d.Edges, edge("e9", "l1", "out", "l2"), edge("e10", "l2", "out", "c"))
		}, "цикл"},
		"duplicate key": {func(d *workflow.Document) { d.Nodes[3].Key = "log1" }, "уже используется"},
		"bad key":       {func(d *workflow.Document) { d.Nodes[3].Key = "1-bad" }, "имя переменной"},
		"duplicate id":  {func(d *workflow.Document) { d.Nodes[3].ID = "l1" }, "повторяющийся ID"},
		"two triggers": {func(d *workflow.Document) {
			d.Nodes = append(d.Nodes, node("t2", nodes.TypeManualTrigger, "trigger2", nil))
		}, "только один"},
		"required param": {func(d *workflow.Document) { d.Nodes[2].Params["message"] = "" }, "обязательно"},
		"bad select":     {func(d *workflow.Document) { d.Nodes[1].Params["operator"] = "regex" }, "недопустимое значение"},
		"schema too new": {func(d *workflow.Document) { d.SchemaVersion = 99 }, "версия схемы"},
		"duplicate edge": {func(d *workflow.Document) { d.Edges = append(d.Edges, edge("e9", "t", "out", "c")) }, "повторяющееся"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := validDoc()
			c.mutate(&d)
			issues := Validate(d, reg)
			if !HasErrors(issues) {
				t.Fatalf("expected an error containing %q, got none", c.want)
			}
			if !strings.Contains(messages(issues), c.want) {
				t.Fatalf("expected %q in:\n%s", c.want, messages(issues))
			}
		})
	}
}

func TestOptionalWhen(t *testing.T) {
	d := validDoc()
	d.Nodes[1].Params = map[string]any{"variable": "{{x}}", "operator": "is_empty"}
	if issues := Validate(d, nodes.NewRegistry()); HasErrors(issues) {
		t.Fatalf("value must be optional for is_empty:\n%s", messages(issues))
	}
}

func TestHTTPParamValidation(t *testing.T) {
	d := validDoc()
	d.Nodes = append(d.Nodes, node("h", nodes.TypeHTTPRequest, "http", map[string]any{"method": "GET", "url": "file:///c:/secret.txt", "timeout": 500.0}))
	d.Edges = append(d.Edges, edge("e9", "l1", "out", "h"))
	msg := messages(Validate(d, nodes.NewRegistry()))
	if !strings.Contains(msg, "http://") || !strings.Contains(msg, "больше") {
		t.Fatalf("expected URL scheme and timeout errors:\n%s", msg)
	}
}

func TestUnreachableIsWarning(t *testing.T) {
	d := validDoc()
	d.Nodes = append(d.Nodes, node("lonely", nodes.TypeLogger, "lonely", map[string]any{"message": "x", "level": "INFO"}))
	issues := Validate(d, nodes.NewRegistry())
	if HasErrors(issues) || len(issues) != 1 || issues[0].Severity != SeverityWarning {
		t.Fatalf("want one warning, got:\n%s", messages(issues))
	}
}

func TestTopoOrderFollowsPosition(t *testing.T) {
	d := validDoc()
	d.Nodes[2].Position = workflow.Position{X: 500, Y: 0}
	d.Nodes[3].Position = workflow.Position{X: 400, Y: 0}
	order, cycle := TopoOrder(d)
	if len(cycle) != 0 || strings.Join(order, ",") != "t,c,l2,l1" {
		t.Fatalf("unexpected order %v (cycle %v)", order, cycle)
	}
}
