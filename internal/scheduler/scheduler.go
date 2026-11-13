// Package scheduler arms the automatic triggers (cron, interval, one-time
// schedule, file watcher, application startup) of active workflows and fires
// runs when they are due.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"veylora/internal/nodes"
	"veylora/internal/workflow"
)

// SourceSchedule is the trigger event source for time-based triggers.
const SourceSchedule = "schedule"

// FireFunc starts a run of a workflow from the given trigger node.
type FireFunc func(workflowID, triggerNodeID string, ev nodes.TriggerEvent)

// LastFiredFunc returns when a trigger last fired (unix ms, 0 = never).
type LastFiredFunc func(workflowID, nodeID string) int64

// TriggerInfo describes an armed trigger for the GUI.
type TriggerInfo struct {
	WorkflowID string `json:"workflowId"`
	NodeID     string `json:"nodeId"`
	NodeName   string `json:"nodeName"`
	Kind       string `json:"kind"`
	// NextRun is the next planned fire time (unix ms), 0 if event-based or finished.
	NextRun int64  `json:"nextRun"`
	Detail  string `json:"detail"`
}

// ErrNoAutomaticTriggers is returned when activating a workflow without automatic triggers.
var ErrNoAutomaticTriggers = errors.New("в Workflow нет автоматических триггеров (Cron, Interval, Schedule, File Watcher, Application Startup, Webhook)")

type registration struct {
	cancel   context.CancelFunc
	triggers map[string]*TriggerInfo
	startup  []string
}

// Scheduler is safe for concurrent use.
type Scheduler struct {
	reg       *nodes.Registry
	fire      FireFunc
	lastFired LastFiredFunc
	logger    *slog.Logger
	now       func() time.Time

	mu      sync.Mutex
	wfs     map[string]*registration
	stopped bool
	wg      sync.WaitGroup
}

// New creates a scheduler.
func New(reg *nodes.Registry, fire FireFunc, lastFired LastFiredFunc, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	if lastFired == nil {
		lastFired = func(string, string) int64 { return 0 }
	}
	return &Scheduler{reg: reg, fire: fire, lastFired: lastFired, logger: logger, now: time.Now, wfs: map[string]*registration{}}
}

type plan struct {
	node workflow.Node
	kind string
	run  func(ctx context.Context, info *TriggerInfo)
	// cleanup releases resources if the plan is never run.
	cleanup func()
}

func cleanupPlans(plans []plan) {
	for _, p := range plans {
		if p.cleanup != nil {
			p.cleanup()
		}
	}
}

// Activate (re)arms all automatic triggers of a workflow. On error nothing is armed.
func (s *Scheduler) Activate(workflowID string, doc workflow.Document, allowedDirs []string) ([]TriggerInfo, error) {
	var plans []plan
	for _, n := range doc.Nodes {
		def, ok := s.reg.Def(n.Type)
		if !ok || !def.IsTrigger() || def.Trigger == nodes.TriggerManual {
			continue
		}
		p, err := s.planFor(workflowID, n, def.Trigger, allowedDirs)
		if err != nil {
			cleanupPlans(plans)
			return nil, fmt.Errorf("«%s»: %w", n.Name, err)
		}
		plans = append(plans, p)
	}
	if len(plans) == 0 {
		return nil, ErrNoAutomaticTriggers
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		cleanupPlans(plans)
		return nil, errors.New("планировщик остановлен")
	}
	if old, ok := s.wfs[workflowID]; ok {
		old.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &registration{cancel: cancel, triggers: map[string]*TriggerInfo{}}
	for _, p := range plans {
		info := &TriggerInfo{WorkflowID: workflowID, NodeID: p.node.ID, NodeName: p.node.Name, Kind: p.kind}
		r.triggers[p.node.ID] = info
		if p.kind == nodes.TriggerStartup {
			info.Detail = "при запуске Veylora"
			r.startup = append(r.startup, p.node.ID)
			continue
		}
		s.wg.Add(1)
		go func(p plan, info *TriggerInfo) {
			defer s.wg.Done()
			p.run(ctx, info)
		}(p, info)
	}
	s.wfs[workflowID] = r
	s.mu.Unlock()
	return s.Triggers(workflowID), nil
}

// Deactivate disarms a workflow's triggers.
func (s *Scheduler) Deactivate(workflowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.wfs[workflowID]; ok {
		r.cancel()
		delete(s.wfs, workflowID)
	}
}

// IsActive reports whether a workflow has armed triggers.
func (s *Scheduler) IsActive(workflowID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.wfs[workflowID]
	return ok
}

// FireStartup fires all Application Startup triggers. Call once after start-up activation.
func (s *Scheduler) FireStartup() {
	s.mu.Lock()
	type item struct{ wf, node string }
	var items []item
	for wf, r := range s.wfs {
		for _, id := range r.startup {
			items = append(items, item{wf, id})
		}
	}
	s.mu.Unlock()
	for _, it := range items {
		s.fire(it.wf, it.node, nodes.TriggerEvent{Source: nodes.TriggerStartup})
	}
}

// Triggers lists armed triggers of a workflow ("" = all workflows), sorted by next run.
func (s *Scheduler) Triggers(workflowID string) []TriggerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []TriggerInfo{}
	for wf, r := range s.wfs {
		if workflowID != "" && wf != workflowID {
			continue
		}
		for _, t := range r.triggers {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].NextRun, out[j].NextRun
		if (a == 0) != (b == 0) {
			return b == 0
		}
		if a != b {
			return a < b
		}
		return out[i].NodeName < out[j].NodeName
	})
	return out
}

// Stop disarms everything and waits for trigger goroutines to exit.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	s.stopped = true
	for id, r := range s.wfs {
		r.cancel()
		delete(s.wfs, id)
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Scheduler) setNext(info *TriggerInfo, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.IsZero() {
		info.NextRun = 0
	} else {
		info.NextRun = t.UnixMilli()
	}
}

func (s *Scheduler) setDetail(info *TriggerInfo, d string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info.Detail = d
}

func (s *Scheduler) planFor(workflowID string, n workflow.Node, kind string, allowedDirs []string) (plan, error) {
	p := plan{node: n, kind: kind}
	switch kind {
	case nodes.TriggerCron:
		sched, err := nodes.ParseCron(nodes.Str(n.Params, "expression", ""))
		if err != nil {
			return p, err
		}
		loc, err := nodes.LoadLocation(nodes.Str(n.Params, "timezone", ""))
		if err != nil {
			return p, err
		}
		expr := nodes.Str(n.Params, "expression", "")
		p.run = func(ctx context.Context, info *TriggerInfo) {
			s.setDetail(info, "cron "+expr+" ("+loc.String()+")")
			for {
				next := sched.Next(s.now().In(loc))
				s.setNext(info, next)
				if !s.waitUntil(ctx, next) {
					return
				}
				s.fire(workflowID, n.ID, nodes.TriggerEvent{Source: SourceSchedule, Data: map[string]any{"scheduledAt": next.Format(time.RFC3339)}})
			}
		}
	case nodes.TriggerInterval:
		d, err := nodes.IntervalDuration(n.Params)
		if err != nil {
			return p, err
		}
		p.run = func(ctx context.Context, info *TriggerInfo) {
			s.setDetail(info, "каждые "+d.String())
			next := s.now().Add(d)
			// Resume the cadence after a restart instead of firing immediately.
			if last := s.lastFired(workflowID, n.ID); last > 0 {
				if cand := time.UnixMilli(last).Add(d); cand.After(s.now()) {
					next = cand
				}
			}
			for {
				s.setNext(info, next)
				if !s.waitUntil(ctx, next) {
					return
				}
				s.fire(workflowID, n.ID, nodes.TriggerEvent{Source: SourceSchedule, Data: map[string]any{"scheduledAt": next.Format(time.RFC3339)}})
				next = next.Add(d)
				if now := s.now(); next.Before(now) {
					next = now.Add(d)
				}
			}
		}
	case nodes.TriggerSchedule:
		at, err := nodes.ScheduleTime(n.Params)
		if err != nil {
			return p, err
		}
		p.run = func(ctx context.Context, info *TriggerInfo) {
			if !at.After(s.now()) {
				detail := "время прошло — запуск не выполняется"
				if last := s.lastFired(workflowID, n.ID); last >= at.UnixMilli() {
					detail = "выполнен " + time.UnixMilli(last).Format("02.01.2006 15:04")
				}
				s.setDetail(info, detail)
				return
			}
			s.setDetail(info, "однократно "+at.Format("02.01.2006 15:04 MST"))
			s.setNext(info, at)
			if !s.waitUntil(ctx, at) {
				return
			}
			s.setNext(info, time.Time{})
			s.setDetail(info, "выполнен "+at.Format("02.01.2006 15:04"))
			s.fire(workflowID, n.ID, nodes.TriggerEvent{Source: SourceSchedule, Data: map[string]any{"scheduledAt": at.Format(time.RFC3339)}})
		}
	case nodes.TriggerFile:
		dir, err := nodes.ResolveAllowedDir(nodes.Str(n.Params, "directory", ""), allowedDirs)
		if err != nil {
			return p, err
		}
		cfg := fileConfig{
			dir:       dir,
			pattern:   nodes.Str(n.Params, "pattern", "*"),
			onCreate:  nodes.Bool(n.Params, "onCreate", true),
			onModify:  nodes.Bool(n.Params, "onModify", false),
			onDelete:  nodes.Bool(n.Params, "onDelete", false),
			recursive: nodes.Bool(n.Params, "recursive", false),
		}
		if cfg.pattern == "" {
			cfg.pattern = "*"
		}
		w, err := newFileWatcher(cfg)
		if err != nil {
			return p, err
		}
		p.cleanup = func() { w.w.Close() }
		p.run = func(ctx context.Context, info *TriggerInfo) {
			s.setDetail(info, "следит за "+dir)
			w.run(ctx, s.logger, func(data map[string]any) {
				s.fire(workflowID, n.ID, nodes.TriggerEvent{Source: nodes.TriggerFile, Data: data})
			})
		}
	case nodes.TriggerStartup:
		p.run = func(context.Context, *TriggerInfo) {}
	case nodes.TriggerWebhook:
		ep := strings.TrimSpace(nodes.Str(n.Params, "endpoint", ""))
		if ep == "" {
			return p, errors.New("эндпоинт вебхука не может быть пустым")
		}
		method := nodes.Str(n.Params, "method", "POST")
		p.run = func(ctx context.Context, info *TriggerInfo) {
			s.setDetail(info, fmt.Sprintf("%s /webhook/%s", method, ep))
			<-ctx.Done()
		}
	default:
		return p, fmt.Errorf("неизвестный тип триггера %q", kind)
	}
	return p, nil
}

// waitUntil sleeps until t; it returns false if ctx was cancelled first.
// Long waits are split so that system sleep / clock changes are picked up.
func (s *Scheduler) waitUntil(ctx context.Context, t time.Time) bool {
	for {
		d := t.Sub(s.now())
		if d <= 0 {
			return true
		}
		if d > time.Minute {
			d = time.Minute
		}
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}
