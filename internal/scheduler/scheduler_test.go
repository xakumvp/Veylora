package scheduler

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

type fired struct {
	mu     sync.Mutex
	events []nodes.TriggerEvent
	nodes  []string
	ch     chan struct{}
}

func newFired() *fired { return &fired{ch: make(chan struct{}, 16)} }

func (f *fired) fire(_ string, nodeID string, ev nodes.TriggerEvent) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.nodes = append(f.nodes, nodeID)
	f.mu.Unlock()
	f.ch <- struct{}{}
}

func (f *fired) wait(t *testing.T, d time.Duration) nodes.TriggerEvent {
	t.Helper()
	select {
	case <-f.ch:
	case <-time.After(d):
		t.Fatal("trigger did not fire")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.events[len(f.events)-1]
}

func doc(nodesList ...workflow.Node) workflow.Document {
	d := workflow.New()
	d.Nodes = nodesList
	return d
}

func node(id, typ string, params map[string]any) workflow.Node {
	return workflow.Node{ID: id, Type: typ, Name: id, Key: id, Params: params}
}

func newSched(t *testing.T, f *fired) *Scheduler {
	s := New(nodes.NewRegistry(), f.fire, nil, nil)
	t.Cleanup(s.Stop)
	return s
}

func TestFileTriggerFires(t *testing.T) {
	dir := t.TempDir()
	f := newFired()
	s := newSched(t, f)
	_, err := s.Activate("wf", doc(node("watch", nodes.TypeFileTrigger, map[string]any{
		"directory": dir, "pattern": "*.txt", "onCreate": true,
	})), []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "ignored.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	ev := f.wait(t, 5*time.Second)
	if ev.Source != nodes.TriggerFile || ev.Data["name"] != "report.txt" || ev.Data["event"] != nodes.FileCreated {
		t.Fatalf("unexpected event %+v", ev)
	}
	// The .log file must have been filtered out.
	time.Sleep(fileDebounce + 200*time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) != 1 {
		t.Fatalf("want exactly one event, got %d: %+v", len(f.events), f.events)
	}
}

func TestFileTriggerRejectsDisallowedDirectory(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	s := newSched(t, newFired())
	d := doc(node("watch", nodes.TypeFileTrigger, map[string]any{"directory": outside}))
	if _, err := s.Activate("wf", d, []string{allowed}); !errors.Is(err, nodes.ErrPathNotAllowed) {
		t.Fatalf("want ErrPathNotAllowed, got %v", err)
	}
	// Path traversal out of the allowed root.
	sneaky := filepath.Join(allowed, "..", filepath.Base(outside))
	d = doc(node("watch", nodes.TypeFileTrigger, map[string]any{"directory": sneaky}))
	if _, err := s.Activate("wf", d, []string{allowed}); !errors.Is(err, nodes.ErrPathNotAllowed) {
		t.Fatalf("traversal must be rejected, got %v", err)
	}
	if s.IsActive("wf") {
		t.Fatal("nothing must be armed after a failed activation")
	}
}

func TestResolveAllowedDirAcceptsSubdirectory(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "inbox")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := nodes.ResolveAllowedDir(sub, []string{root}); err != nil {
		t.Fatalf("subdirectory must be allowed: %v", err)
	}
	if _, err := nodes.ResolveAllowedDir("relative/path", []string{root}); err == nil {
		t.Fatal("relative paths must be rejected")
	}
}

func TestCronNextRunInTimezone(t *testing.T) {
	s := newSched(t, newFired())
	fixed := time.Date(2026, 3, 2, 8, 30, 0, 0, time.UTC) // Monday 08:30 UTC = 11:30 Moscow
	s.now = func() time.Time { return fixed }
	triggers, err := s.Activate("wf", doc(node("cron", nodes.TypeCronTrigger, map[string]any{
		"expression": "0 12 * * 1-5", "timezone": "Europe/Moscow",
	})), nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	triggers = s.Triggers("wf")
	want := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC).UnixMilli() // 12:00 Moscow
	if len(triggers) != 1 || triggers[0].NextRun != want {
		t.Fatalf("next run = %v, want %v", time.UnixMilli(triggers[0].NextRun).UTC(), time.UnixMilli(want).UTC())
	}
}

func TestPastScheduleIsNotRun(t *testing.T) {
	f := newFired()
	s := newSched(t, f)
	_, err := s.Activate("wf", doc(node("once", nodes.TypeScheduleTrigger, map[string]any{"at": "2020-01-01T10:00"})), nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	tr := s.Triggers("wf")
	if len(tr) != 1 || tr[0].NextRun != 0 || tr[0].Detail == "" {
		t.Fatalf("unexpected trigger state %+v", tr)
	}
	select {
	case <-f.ch:
		t.Fatal("past schedule must not fire")
	default:
	}
}

func TestStartupAndValidation(t *testing.T) {
	f := newFired()
	s := newSched(t, f)
	if _, err := s.Activate("wf", doc(node("m", nodes.TypeManualTrigger, nil)), nil); !errors.Is(err, ErrNoAutomaticTriggers) {
		t.Fatalf("manual-only workflow must not activate, got %v", err)
	}
	if _, err := s.Activate("wf", doc(node("c", nodes.TypeCronTrigger, map[string]any{"expression": "not a cron"})), nil); err == nil {
		t.Fatal("invalid cron must be rejected")
	}
	if _, err := s.Activate("wf", doc(node("boot", nodes.TypeStartupTrigger, nil)), nil); err != nil {
		t.Fatal(err)
	}
	s.FireStartup()
	if ev := f.wait(t, time.Second); ev.Source != nodes.TriggerStartup {
		t.Fatalf("source = %s", ev.Source)
	}
	s.Deactivate("wf")
	if s.IsActive("wf") || len(s.Triggers("wf")) != 0 {
		t.Fatal("deactivate failed")
	}
}

func TestIntervalResumesAfterRestart(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-2 * time.Minute)
	s := New(nodes.NewRegistry(), newFired().fire, func(string, string) int64 { return last.UnixMilli() }, nil)
	t.Cleanup(s.Stop)
	s.now = func() time.Time { return now }
	if _, err := s.Activate("wf", doc(node("every", nodes.TypeIntervalTrigger, map[string]any{"every": 5.0, "unit": "min"})), nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	tr := s.Triggers("wf")
	if want := last.Add(5 * time.Minute).UnixMilli(); tr[0].NextRun != want {
		t.Fatalf("next = %v, want %v", time.UnixMilli(tr[0].NextRun).UTC(), time.UnixMilli(want).UTC())
	}
}
