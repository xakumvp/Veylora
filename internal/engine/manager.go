package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrAlreadyRunning is returned when a run is requested while another is in progress.
var ErrAlreadyRunning = errors.New("Workflow уже выполняется — дождитесь завершения или нажмите Stop")

// Manager owns the single active run (MVP: one run at a time) and its cancellation.
type Manager struct {
	engine *Engine

	mu      sync.Mutex
	cancel  context.CancelFunc
	runID   string
	done    chan struct{}
	closing bool
}

// NewManager creates a run manager.
func NewManager(e *Engine) *Manager {
	return &Manager{engine: e}
}

// Start validates the request and launches it in the background.
// Validation errors are returned synchronously so the GUI can show them.
func (m *Manager) Start(req Request, sink Sink) (string, error) {
	if _, err := m.engine.Prepare(req.Doc); err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return "", errors.New("приложение завершает работу")
	}
	if m.runID != "" {
		return "", ErrAlreadyRunning
	}
	if req.RunID == "" {
		req.RunID = uuid.NewString()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.cancel, m.runID, m.done = cancel, req.RunID, done

	go func() {
		defer close(done)
		defer cancel()
		m.engine.Run(ctx, req, sink)
		m.mu.Lock()
		if m.runID == req.RunID {
			m.runID, m.cancel, m.done = "", nil, nil
		}
		m.mu.Unlock()
	}()
	return req.RunID, nil
}

// Stop cancels the active run, if any. It returns immediately.
func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel == nil {
		return false
	}
	m.cancel()
	return true
}

// ActiveRunID returns the ID of the running workflow, or "".
func (m *Manager) ActiveRunID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runID
}

// Wait blocks until the active run (if any) finishes or the timeout passes.
func (m *Manager) Wait(timeout time.Duration) bool {
	m.mu.Lock()
	done := m.done
	m.mu.Unlock()
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Shutdown refuses new runs, cancels the active one and waits for it to finish
// so its CANCELLED status is persisted before the app exits.
func (m *Manager) Shutdown(timeout time.Duration) {
	m.mu.Lock()
	m.closing = true
	m.mu.Unlock()
	m.Stop()
	m.Wait(timeout)
}
