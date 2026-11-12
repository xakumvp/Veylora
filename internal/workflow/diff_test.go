package workflow

import "testing"

func TestDiff(t *testing.T) {
	old := New()
	old.Nodes = []Node{
		{ID: "a", Type: "trigger.manual", Name: "Start", Key: "trigger", Params: map[string]any{}},
		{ID: "b", Type: "logging.logger", Name: "Log", Key: "log", Params: map[string]any{"message": "hi", "level": "INFO"}},
		{ID: "c", Type: "utility.delay", Name: "Wait", Key: "delay", Params: map[string]any{"duration": 1.0}},
	}
	old.Edges = []Edge{{ID: "e1", Source: "a", SourceHandle: "out", Target: "b", TargetHandle: "in"}}

	cur := New()
	cur.Nodes = []Node{
		{ID: "a", Type: "trigger.manual", Name: "Start", Key: "trigger", Params: map[string]any{}},
		{ID: "b", Type: "logging.logger", Name: "Log", Key: "log", Params: map[string]any{"message": "bye", "level": "INFO"}},
		{ID: "d", Type: "http.request", Name: "Call", Key: "http", Params: map[string]any{}},
	}
	cur.Edges = []Edge{{ID: "x", Source: "a", SourceHandle: "out", Target: "d", TargetHandle: "in"}}
	cur.Viewport = &Viewport{X: 10, Zoom: 2}

	d := Diff(old, cur)
	if len(d.AddedNodes) != 1 || d.AddedNodes[0].ID != "d" || len(d.RemovedNodes) != 1 || d.RemovedNodes[0].ID != "c" {
		t.Fatalf("nodes: %+v", d)
	}
	if len(d.ChangedNodes) != 1 || d.ChangedNodes[0].Fields[0] != "params.message" {
		t.Fatalf("changed: %+v", d.ChangedNodes)
	}
	if d.AddedEdges != 1 || d.RemovedEdges != 1 || d.Identical {
		t.Fatalf("edges: %+v", d)
	}
	if !Diff(cur, cur).Identical {
		t.Fatal("document must be identical to itself")
	}
	moved := cur
	moved.Viewport = &Viewport{Zoom: 0.5}
	if Fingerprint(moved) != Fingerprint(cur) {
		t.Fatal("viewport must not affect the fingerprint")
	}
}
