package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrAlreadyRunning is returned when a workflow is started while a previous run of it is in progress.
var ErrAlreadyRunning = errors.New("этот Workflow уже выполняется — дождитесь завершения или нажмите Stop")

// ErrTooManyRuns is returned when the global concurrency limit is reached.
var ErrTooManyRuns = errors.New("достигнут лимит одновременных запусков (Settings)")

// DefaultMaxConcurrent is used when no limit is configured.
const DefaultMaxConcurrent = 4

// ActiveRun describes a run in progress.
type ActiveRun struct {
	RunID      string `json:"runId"`
	WorkflowID string `json:"workflowId"`
	StartedAt  int64  `json:"startedAt"`
}

type activeRun struct {
	info   ActiveRun
	cancel context.CancelFunc
	done   chan struct{}
}

// Manager owns all active runs: it enforces the concurrency limit, prevents
// overlapping runs of the same workflow and handles cancellation.
type Manager struct {
	engine *Engine

	mu            sync.Mutex
	runs          map[string]*activeRun // by run ID
	maxConcurrent int
	closing       bool
}

// NewManager creates a run manager.
func NewManager(e *Engine) *Manager {
	return &Manager{engine: e, runs: map[string]*activeRun{}, maxConcurrent: DefaultMaxConcurrent}
}

// SetMaxConcurrent changes the global limit of simultaneous runs.
func (m *Manager) SetMaxConcurrent(n int) {
	if n < 1 {
		n = 1
	}
	m.mu.Lock()
	m.maxConcurrent = n
	m.mu.Unlock()
}

// Start validates the request and launches it in the background.
// Validation errors are returned synchronously so the GUI can show them.
func (m *Manager) Start(req Request, sink Sink) (string, error) {
	if _, err := m.engine.Prepare(req.Doc); err != nil {
		return "", err
	}
	if _, err := m.engine.PickTrigger(req.Doc, req.TriggerNodeID); err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return "", errors.New("приложение завершает работу")
	}
	for _, r := range m.runs {
		if r.info.WorkflowID == req.WorkflowID {
			return "", ErrAlreadyRunning
		}
	}
	if len(m.runs) >= m.maxConcurrent {
		return "", fmt.Errorf("%w: %d", ErrTooManyRuns, m.maxConcurrent)
	}
	if req.RunID == "" {
		req.RunID = uuid.NewString()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &activeRun{
		info:   ActiveRun{RunID: req.RunID, WorkflowID: req.WorkflowID, StartedAt: time.Now().UnixMilli()},
		cancel: cancel,
		done:   make(chan struct{}),
	}
	m.runs[req.RunID] = r

	go func() {
		defer close(r.done)
		defer cancel()
		defer func() {
			m.mu.Lock()
			delete(m.runs, req.RunID)
			m.mu.Unlock()
		}()
		m.engine.Run(ctx, req, sink)
	}()
	return req.RunID, nil
}

// Stop cancels one run. It returns false if the run is not active.
func (m *Manager) Stop(runID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[runID]
	if ok {
		r.cancel()
	}
	return ok
}

// StopWorkflow cancels the active run of a workflow.
func (m *Manager) StopWorkflow(workflowID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	stopped := false
	for _, r := range m.runs {
		if r.info.WorkflowID == workflowID {
			r.cancel()
			stopped = true
		}
	}
	return stopped
}

// Active lists runs in progress, oldest first.
func (m *Manager) Active() []ActiveRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ActiveRun, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, r.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out
}

// IsRunning reports whether a workflow has an active run.
func (m *Manager) IsRunning(workflowID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.info.WorkflowID == workflowID {
			return true
		}
	}
	return false
}

// Wait blocks until all active runs finish or the timeout passes.
func (m *Manager) Wait(timeout time.Duration) bool {
	m.mu.Lock()
	dones := make([]chan struct{}, 0, len(m.runs))
	for _, r := range m.runs {
		dones = append(dones, r.done)
	}
	m.mu.Unlock()
	deadline := time.After(timeout)
	for _, d := range dones {
		select {
		case <-d:
		case <-deadline:
			return false
		}
	}
	return true
}

// Shutdown refuses new runs, cancels active ones and waits for them to finish
// so their CANCELLED status is persisted before the app exits.
func (m *Manager) Shutdown(timeout time.Duration) {
	m.mu.Lock()
	m.closing = true
	for _, r := range m.runs {
		r.cancel()
	}
	m.mu.Unlock()
	m.Wait(timeout)
}
