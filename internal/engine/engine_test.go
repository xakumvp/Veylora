package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"veylora/internal/events"
	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

type recSink struct {
	mu       sync.Mutex
	statuses map[string][]string
	logs     []events.Log
	outputs  map[string]any
	final    events.RunEvent
	started  chan struct{}
	once     sync.Once
}

func newSink() *recSink {
	return &recSink{statuses: map[string][]string{}, outputs: map[string]any{}, started: make(chan struct{})}
}

func (s *recSink) RunStarted(events.RunEvent) {}
func (s *recSink) NodeUpdated(ev events.NodeEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[ev.NodeID] = append(s.statuses[ev.NodeID], ev.Status)
	if ev.Output != nil {
		s.outputs[ev.NodeID] = ev.Output
	}
	if ev.Status == events.StatusRunning {
		s.once.Do(func() {})
	}
}
func (s *recSink) Log(l events.Log) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, l)
}
func (s *recSink) RunFinished(ev events.RunEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.final = ev
}

func (s *recSink) last(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.statuses[id]
	if len(st) == 0 {
		return ""
	}
	return st[len(st)-1]
}

func (s *recSink) logText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, l := range s.logs {
		b.WriteString(l.Level + " " + l.Message + "\n")
	}
	return b.String()
}

func n(id, typ string, x float64, params map[string]any) workflow.Node {
	return workflow.Node{ID: id, Type: typ, Name: id, Key: id, Params: params, Position: workflow.Position{X: x}}
}

func e(src, sh, tgt string) workflow.Edge {
	return workflow.Edge{ID: src + "-" + sh + "-" + tgt, Source: src, SourceHandle: sh, Target: tgt, TargetHandle: "in"}
}

func branchingDoc(url string) workflow.Document {
	d := workflow.New()
	d.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{"runName": "test"}),
		n("http", nodes.TypeHTTPRequest, 1, map[string]any{"method": "GET", "url": url, "timeout": 5.0}),
		n("cond", nodes.TypeCondition, 2, map[string]any{"variable": "{{http.status}}", "operator": "equals", "value": "200"}),
		n("yes", nodes.TypeLogger, 3, map[string]any{"message": "msg={{http.body.message}}", "level": "INFO"}),
		n("no", nodes.TypeLogger, 3, map[string]any{"message": "failed", "level": "ERROR"}),
		n("after", nodes.TypeLogger, 4, map[string]any{"message": "after yes", "level": "DEBUG"}),
	}
	d.Edges = []workflow.Edge{
		e("trigger", "out", "http"), e("http", "out", "cond"),
		e("cond", "true", "yes"), e("cond", "false", "no"), e("yes", "out", "after"),
	}
	return d
}

func TestRunBranchingAndVariables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"OK"}`))
	}))
	defer srv.Close()

	eng := New(nodes.NewRegistry(), nil)
	sink := newSink()
	final := eng.Run(context.Background(), Request{RunID: "r1", WorkflowID: "w1", Doc: branchingDoc(srv.URL), Env: nodes.Env{AllowLocalNetwork: true}}, sink)

	if final.Status != events.StatusSuccess {
		t.Fatalf("run status %s (%s)\n%s", final.Status, final.Error, sink.logText())
	}
	want := map[string]string{"trigger": "SUCCESS", "http": "SUCCESS", "cond": "SUCCESS", "yes": "SUCCESS", "no": "SKIPPED", "after": "SUCCESS"}
	for id, st := range want {
		if got := sink.last(id); got != st {
			t.Errorf("%s: status %s, want %s", id, got, st)
		}
	}
	if !strings.Contains(sink.logText(), "msg=OK") {
		t.Fatalf("variable not passed to Logger:\n%s", sink.logText())
	}
	if sink.statuses["http"][0] != events.StatusWaiting {
		t.Errorf("nodes must start WAITING, got %v", sink.statuses["http"])
	}
}

func TestRunFalseBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	sink := newSink()
	New(nodes.NewRegistry(), nil).Run(context.Background(), Request{RunID: "r", Doc: branchingDoc(srv.URL), Env: nodes.Env{AllowLocalNetwork: true}}, sink)
	if sink.last("yes") != "SKIPPED" || sink.last("after") != "SKIPPED" || sink.last("no") != "SUCCESS" {
		t.Fatalf("wrong branch: yes=%s after=%s no=%s", sink.last("yes"), sink.last("after"), sink.last("no"))
	}
}

func TestRunFailureStopsPath(t *testing.T) {
	sink := newSink()
	doc := branchingDoc("http://127.0.0.1:1/") // local network blocked by default
	final := New(nodes.NewRegistry(), nil).Run(context.Background(), Request{RunID: "r", Doc: doc}, sink)
	if final.Status != events.StatusFailed {
		t.Fatalf("want FAILED, got %s", final.Status)
	}
	if sink.last("http") != "FAILED" || sink.last("cond") != "SKIPPED" {
		t.Fatalf("http=%s cond=%s", sink.last("http"), sink.last("cond"))
	}
	if !strings.Contains(final.Error, "локальной сети") {
		t.Fatalf("error should explain the block: %s", final.Error)
	}
}

func TestInvalidWorkflowDoesNotStart(t *testing.T) {
	doc := workflow.New()
	doc.Nodes = []workflow.Node{n("log", nodes.TypeLogger, 0, map[string]any{"message": "x", "level": "INFO"})}
	m := NewManager(New(nodes.NewRegistry(), nil))
	_, err := m.Start(Request{Doc: doc}, newSink())
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
}

func TestStopCancelsDelay(t *testing.T) {
	doc := workflow.New()
	doc.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("wait", nodes.TypeDelay, 1, map[string]any{"duration": 30.0, "unit": "s"}),
		n("log", nodes.TypeLogger, 2, map[string]any{"message": "never", "level": "INFO"}),
	}
	doc.Edges = []workflow.Edge{e("trigger", "out", "wait"), e("wait", "out", "log")}

	m := NewManager(New(nodes.NewRegistry(), nil))
	sink := newSink()
	if _, err := m.Start(Request{Doc: doc}, sink); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(Request{Doc: doc}, newSink()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second start must be refused, got %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for sink.last("wait") != events.StatusRunning && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	if !m.Stop() {
		t.Fatal("Stop reported nothing running")
	}
	if !m.Wait(2 * time.Second) {
		t.Fatal("run did not stop")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation too slow")
	}
	if sink.final.Status != events.StatusCancelled || sink.last("wait") != "CANCELLED" || sink.last("log") != "CANCELLED" {
		t.Fatalf("final=%s wait=%s log=%s", sink.final.Status, sink.last("wait"), sink.last("log"))
	}
	if m.ActiveRunID() != "" {
		t.Fatal("manager still reports an active run")
	}
}

func TestSecretsAreMaskedInLogs(t *testing.T) {
	doc := workflow.New()
	doc.Nodes = []workflow.Node{
		n("trigger", nodes.TypeManualTrigger, 0, map[string]any{}),
		n("http", nodes.TypeHTTPRequest, 1, map[string]any{
			"method": "GET", "url": "http://127.0.0.1:1/?api_key=TOPSECRET123", "timeout": 2.0,
			"headers": []any{map[string]any{"key": "Authorization", "value": "Bearer hunter2-token"}},
		}),
		n("log", nodes.TypeLogger, 2, map[string]any{"message": "x", "level": "INFO"}),
	}
	doc.Edges = []workflow.Edge{e("trigger", "out", "http"), e("http", "out", "log")}
	sink := newSink()
	New(nodes.NewRegistry(), nil).Run(context.Background(), Request{RunID: "r", Doc: doc}, sink)
	text := sink.logText()
	if strings.Contains(text, "TOPSECRET123") || strings.Contains(text, "hunter2-token") {
		t.Fatalf("secret leaked into logs:\n%s", text)
	}
}

func TestPanicInNodeIsContained(t *testing.T) {
	out := previewOutput(map[string]any{"body": strings.Repeat("a", maxEventOutputBytes+10)})
	m, ok := out.(map[string]any)
	if !ok || !strings.Contains(m["body"].(string), "обрезано") {
		t.Fatalf("large output was not truncated: %T", out)
	}
}
