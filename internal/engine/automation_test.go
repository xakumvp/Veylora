package engine

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"veylora/internal/events"
	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

// flaky fails until attempt n, then succeeds.
type flaky struct {
	calls *int32
	okAt  int32
	block bool
}

func (f flaky) Definition() nodes.Definition {
	return nodes.Definition{Type: "test.flaky", Name: "Flaky", Category: "Utilities",
		Inputs: []nodes.Port{{ID: "in", Kind: nodes.PortKindFlow}}, Outputs: []nodes.Port{{ID: "out", Kind: nodes.PortKindFlow}}}
}
func (f flaky) Validate(map[string]any) []nodes.FieldError { return nil }
func (f flaky) Execute(ctx context.Context, _ *nodes.Input) (nodes.Result, error) {
	c := atomic.AddInt32(f.calls, 1)
	if f.block {
		<-ctx.Done()
		return nodes.Result{}, ctx.Err()
	}
	if c < f.okAt {
		return nodes.Result{}, errors.New("temporary failure")
	}
	return nodes.Result{Output: map[string]any{"calls": float64(c)}}, nil
}

func newTestEngine(extra ...nodes.Executor) *Engine {
	reg := nodes.NewRegistryWith(extra...)
	e := New(reg, nil)
	e.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() } // no real waiting
	return e
}

func linear(mid workflow.Node) workflow.Document {
	d := workflow.New()
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		mid,
		n("after", nodes.TypeLogger, 2, map[string]any{"message": "after {{mid.error}}", "level": "INFO"}),
	}
	d.Edges = []workflow.Edge{e("trigger", "out", "mid"), e("mid", "out", "after")}
	return d
}

func TestRetrySucceedsOnThirdAttempt(t *testing.T) {
	var calls int32
	eng := newTestEngine(flaky{calls: &calls, okAt: 3})
	mid := n("mid", "test.flaky", 1, map[string]any{})
	mid.Settings = &workflow.NodeSettings{Retry: workflow.Retry{MaxAttempts: 5, DelayMs: 10, Backoff: true}}
	sink := newSink()
	final := eng.Run(context.Background(), Request{RunID: "r", Doc: linear(mid)}, sink)
	if final.Status != events.StatusSuccess || calls != 3 {
		t.Fatalf("status=%s calls=%d\n%s", final.Status, calls, sink.logText())
	}
	if !strings.Contains(sink.logText(), "Попытка 2 из 5") {
		t.Fatalf("retry not logged:\n%s", sink.logText())
	}
}

func TestRetryExhaustedFails(t *testing.T) {
	var calls int32
	eng := newTestEngine(flaky{calls: &calls, okAt: 100})
	mid := n("mid", "test.flaky", 1, map[string]any{})
	mid.Settings = &workflow.NodeSettings{Retry: workflow.Retry{MaxAttempts: 3}}
	final := eng.Run(context.Background(), Request{RunID: "r", Doc: linear(mid)}, newSink())
	if final.Status != events.StatusFailed || calls != 3 {
		t.Fatalf("status=%s calls=%d", final.Status, calls)
	}
}

func TestRetryDelayBackoff(t *testing.T) {
	r := workflow.Retry{DelayMs: 100, Backoff: true, MaxDelayMs: 350}
	got := []time.Duration{retryDelay(r, 1), retryDelay(r, 2), retryDelay(r, 3)}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 350 * time.Millisecond}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("attempt %d: %v, want %v", i+1, got[i], want[i])
		}
	}
}

func TestContinueOnError(t *testing.T) {
	var calls int32
	eng := newTestEngine(flaky{calls: &calls, okAt: 100})
	mid := n("mid", "test.flaky", 1, map[string]any{})
	mid.Settings = &workflow.NodeSettings{OnError: workflow.OnErrorContinue}
	sink := newSink()
	final := eng.Run(context.Background(), Request{RunID: "r", Doc: linear(mid)}, sink)
	if final.Status != events.StatusSuccess || sink.last("after") != "SUCCESS" {
		t.Fatalf("status=%s after=%s", final.Status, sink.last("after"))
	}
	if !strings.Contains(sink.logText(), "after temporary failure") {
		t.Fatalf("error variable not available downstream:\n%s", sink.logText())
	}
}

func TestErrorOutput(t *testing.T) {
	var calls int32
	eng := newTestEngine(flaky{calls: &calls, okAt: 100})
	d := workflow.New()
	mid := n("mid", "test.flaky", 1, map[string]any{})
	mid.Settings = &workflow.NodeSettings{OnError: workflow.OnErrorErrorOutput}
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		mid,
		n("ok", nodes.TypeLogger, 2, map[string]any{"message": "ok", "level": "INFO"}),
		n("handler", nodes.TypeLogger, 2, map[string]any{"message": "handled: {{mid.error}}", "level": "WARNING"}),
	}
	d.Edges = []workflow.Edge{e("trigger", "out", "mid"), e("mid", "out", "ok"), e("mid", nodes.ErrorPortID, "handler")}
	sink := newSink()
	final := eng.Run(context.Background(), Request{RunID: "r", Doc: d}, sink)
	if final.Status != events.StatusSuccess {
		t.Fatalf("handled error must not fail the run: %s (%s)", final.Status, final.Error)
	}
	if sink.last("ok") != "SKIPPED" || sink.last("handler") != "SUCCESS" {
		t.Fatalf("ok=%s handler=%s", sink.last("ok"), sink.last("handler"))
	}
	if !strings.Contains(sink.logText(), "handled: temporary failure") {
		t.Fatalf("error details missing:\n%s", sink.logText())
	}
}

func TestNodeTimeout(t *testing.T) {
	var calls int32
	eng := newTestEngine(flaky{calls: &calls, block: true})
	mid := n("mid", "test.flaky", 1, map[string]any{})
	mid.Settings = &workflow.NodeSettings{TimeoutSec: 0.05}
	sink := newSink()
	final := eng.Run(context.Background(), Request{RunID: "r", Doc: linear(mid)}, sink)
	if final.Status != events.StatusFailed || !strings.Contains(final.Error, "таймаут Node") {
		t.Fatalf("status=%s error=%q", final.Status, final.Error)
	}
	if sink.last("mid") != "FAILED" {
		t.Fatalf("mid=%s", sink.last("mid"))
	}
}

func TestWorkflowTimeout(t *testing.T) {
	d := workflow.New()
	d.Settings.TimeoutSec = 0.1
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("wait", nodes.TypeDelay, 1, map[string]any{"duration": 10.0, "unit": "s"}),
		n("log", nodes.TypeLogger, 2, map[string]any{"message": "x", "level": "INFO"}),
	}
	d.Edges = []workflow.Edge{e("trigger", "out", "wait"), e("wait", "out", "log")}
	start := time.Now()
	final := New(nodes.NewRegistry(), nil).Run(context.Background(), Request{RunID: "r", Doc: d}, newSink())
	if final.Status != events.StatusTimeout || time.Since(start) > 2*time.Second {
		t.Fatalf("status=%s after %v", final.Status, time.Since(start))
	}
}

func TestStopWorkflowNode(t *testing.T) {
	d := workflow.New()
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("stop", nodes.TypeStopWorkflow, 1, map[string]any{"status": "failed", "message": "нет данных для {{workflow.name}}"}),
		n("never", nodes.TypeLogger, 2, map[string]any{"message": "x", "level": "INFO"}),
	}
	d.Edges = []workflow.Edge{e("trigger", "out", "stop")}
	sink := newSink()
	final := New(nodes.NewRegistry(), nil).Run(context.Background(), Request{RunID: "r", WorkflowName: "Отчёт", Doc: d}, sink)
	if final.Status != events.StatusFailed || final.Error != "нет данных для Отчёт" {
		t.Fatalf("status=%s error=%q", final.Status, final.Error)
	}
	if sink.last("never") != "SKIPPED" {
		t.Fatalf("never=%s", sink.last("never"))
	}
}

func TestSystemVariables(t *testing.T) {
	d := workflow.New()
	d.Variables = []workflow.Variable{{Key: "greeting", Value: "Привет"}, {Key: "api_token", Value: "super-secret-123"}}
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("log", nodes.TypeLogger, 1, map[string]any{"level": "INFO",
			"message": "{{variables.greeting}} {{workflow.name}} {{execution.id}} {{execution.trigger}} {{env.HOME_TEST}} {{variables.api_token}}"}),
	}
	d.Edges = []workflow.Edge{e("trigger", "out", "log")}
	sink := newSink()
	New(nodes.NewRegistry(), nil).Run(context.Background(), Request{RunID: "run-42", WorkflowName: "Бэкап", Doc: d,
		EnvVars: map[string]string{"HOME_TEST": "/home/x"}}, sink)
	text := sink.logText()
	if !strings.Contains(text, "Привет Бэкап run-42 manual /home/x") {
		t.Fatalf("variables not resolved:\n%s", text)
	}
	if strings.Contains(text, "super-secret-123") {
		t.Fatalf("secret workflow variable leaked:\n%s", text)
	}
}

func TestStartsFromChosenTrigger(t *testing.T) {
	d := workflow.New()
	d.Nodes = []workflow.Node{
		n("manual", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("cron", nodes.TypeCronTrigger, 0, map[string]any{"expression": "@daily"}),
		n("a", nodes.TypeLogger, 1, map[string]any{"message": "from manual", "level": "INFO"}),
		n("b", nodes.TypeLogger, 1, map[string]any{"message": "from cron {{cron.scheduledAt}}", "level": "INFO"}),
	}
	d.Edges = []workflow.Edge{e("manual", "out", "a"), e("cron", "out", "b")}
	sink := newSink()
	final := New(nodes.NewRegistry(), nil).Run(context.Background(), Request{RunID: "r", Doc: d, TriggerNodeID: "cron",
		Event: nodes.TriggerEvent{Source: "schedule", Data: map[string]any{"scheduledAt": "2026-01-01T09:00:00Z"}}}, sink)
	if final.Status != events.StatusSuccess || final.Trigger != "schedule" {
		t.Fatalf("status=%s trigger=%s", final.Status, final.Trigger)
	}
	if sink.last("manual") != "SKIPPED" || sink.last("a") != "SKIPPED" || sink.last("b") != "SUCCESS" {
		t.Fatalf("manual=%s a=%s b=%s", sink.last("manual"), sink.last("a"), sink.last("b"))
	}
	if !strings.Contains(sink.logText(), "from cron 2026-01-01T09:00:00Z") {
		t.Fatalf("trigger payload missing:\n%s", sink.logText())
	}
}

func TestConcurrencyLimit(t *testing.T) {
	doc := func() workflow.Document {
		d := workflow.New()
		d.Nodes = []workflow.Node{
			n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
			n("wait", nodes.TypeDelay, 1, map[string]any{"duration": 30.0, "unit": "s"}),
		}
		d.Edges = []workflow.Edge{e("trigger", "out", "wait")}
		return d
	}
	m := NewManager(New(nodes.NewRegistry(), nil))
	m.SetMaxConcurrent(2)
	defer m.Shutdown(2 * time.Second)
	for _, wf := range []string{"a", "b"} {
		if _, err := m.Start(Request{WorkflowID: wf, Doc: doc()}, newSink()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Start(Request{WorkflowID: "c", Doc: doc()}, newSink()); !errors.Is(err, ErrTooManyRuns) {
		t.Fatalf("third run must hit the limit, got %v", err)
	}
	if !m.StopWorkflow("a") {
		t.Fatal("stop by workflow failed")
	}
}
