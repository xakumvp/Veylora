package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"veylora/internal/events"
	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

type mapLoader map[string]workflow.Document

func (m mapLoader) LoadWorkflow(_ context.Context, id string) (string, workflow.Document, error) {
	d, ok := m[id]
	if !ok {
		return "", workflow.Document{}, errors.New("not found")
	}
	return "child-" + id, d, nil
}

func callDoc(target string, timeout float64) workflow.Document {
	d := workflow.New()
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("sub", nodes.TypeSubflow, 1, map[string]any{"workflowId": target, "timeout": timeout,
			"input": []any{map[string]any{"key": "who", "value": "мир"}}}),
		n("log", nodes.TypeLogger, 2, map[string]any{"message": "child said {{sub.outputs.greet.message}}", "level": "INFO"}),
	}
	d.Edges = []workflow.Edge{e("trigger", "out", "sub"), e("sub", "out", "log")}
	return d
}

func childDoc() workflow.Document {
	d := workflow.New()
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("greet", nodes.TypeLogger, 1, map[string]any{"message": "привет, {{trigger.input.who}}", "level": "INFO"}),
	}
	d.Edges = []workflow.Edge{e("trigger", "out", "greet")}
	return d
}

func TestSubflowPassesInputAndReturnsOutputs(t *testing.T) {
	eng := New(nodes.NewRegistry(), nil)
	eng.Loader = mapLoader{"child": childDoc()}
	sink := newSink()
	final := eng.Run(context.Background(), Request{RunID: "r", WorkflowID: "parent", Doc: callDoc("child", 10)}, sink)
	if final.Status != events.StatusSuccess {
		t.Fatalf("status %s: %s\n%s", final.Status, final.Error, sink.logText())
	}
	text := sink.logText()
	forwarded := false
	for _, l := range sink.logs {
		if strings.HasPrefix(l.NodeName, "↳ child-child") && l.RunID == "r" {
			forwarded = true
		}
	}
	if !strings.Contains(text, "child said привет, мир") || !forwarded {
		t.Fatalf("subflow data not propagated:\n%s", text)
	}
}

func TestSubflowRecursionIsBlocked(t *testing.T) {
	eng := New(nodes.NewRegistry(), nil)
	loop := callDoc("parent", 10)
	eng.Loader = mapLoader{"parent": loop}
	final := eng.Run(context.Background(), Request{RunID: "r", WorkflowID: "parent", Doc: loop}, newSink())
	if final.Status != events.StatusFailed || !strings.Contains(final.Error, "рекурсивный") {
		t.Fatalf("status %s: %s", final.Status, final.Error)
	}
}

func TestSubflowFailurePropagates(t *testing.T) {
	bad := workflow.New()
	bad.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("stop", nodes.TypeStopWorkflow, 1, map[string]any{"status": "failed", "message": "нет данных"}),
	}
	bad.Edges = []workflow.Edge{e("trigger", "out", "stop")}
	eng := New(nodes.NewRegistry(), nil)
	eng.Loader = mapLoader{"bad": bad}
	final := eng.Run(context.Background(), Request{RunID: "r", WorkflowID: "parent", Doc: callDoc("bad", 10)}, newSink())
	if final.Status != events.StatusFailed || !strings.Contains(final.Error, "нет данных") {
		t.Fatalf("status %s: %s", final.Status, final.Error)
	}
}

func TestSubflowTimeout(t *testing.T) {
	slow := workflow.New()
	slow.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("wait", nodes.TypeDelay, 1, map[string]any{"duration": 30.0, "unit": "s"}),
	}
	slow.Edges = []workflow.Edge{e("trigger", "out", "wait")}
	eng := New(nodes.NewRegistry(), nil)
	eng.Loader = mapLoader{"slow": slow}
	start := time.Now()
	final := eng.Run(context.Background(), Request{RunID: "r", WorkflowID: "parent", Doc: callDoc("slow", 1)}, newSink())
	if final.Status != events.StatusFailed || !strings.Contains(final.Error, "Subflow Timeout") || time.Since(start) > 5*time.Second {
		t.Fatalf("status %s: %s after %v", final.Status, final.Error, time.Since(start))
	}
}
