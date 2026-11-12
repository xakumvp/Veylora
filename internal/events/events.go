// Package events defines the real-time messages the backend sends to the GUI.
package events

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Event names. Keep in sync with frontend/src/services/events.ts.
const (
	RunStarted  = "run:started"
	RunFinished = "run:finished"
	NodeStatus  = "node:status"
	LogEntry    = "log:entry"
)

// Node states.
const (
	StatusIdle      = "IDLE"
	StatusWaiting   = "WAITING"
	StatusRunning   = "RUNNING"
	StatusSuccess   = "SUCCESS"
	StatusFailed    = "FAILED"
	StatusSkipped   = "SKIPPED"
	StatusCancelled = "CANCELLED"
)

// RunEvent describes the lifecycle of a whole workflow run.
type RunEvent struct {
	RunID      string `json:"runId"`
	WorkflowID string `json:"workflowId"`
	Status     string `json:"status"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

// NodeEvent is a node state change.
type NodeEvent struct {
	RunID      string `json:"runId"`
	NodeID     string `json:"nodeId"`
	Status     string `json:"status"`
	Branch     string `json:"branch,omitempty"`
	Error      string `json:"error,omitempty"`
	Output     any    `json:"output,omitempty"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// Log is a line in the Execution Panel.
type Log struct {
	ID        int64  `json:"id,omitempty"`
	RunID     string `json:"runId"`
	NodeID    string `json:"nodeId,omitempty"`
	NodeName  string `json:"nodeName,omitempty"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// Emitter sends named events to the GUI.
type Emitter interface {
	Emit(name string, payload any)
}

// WailsEmitter forwards events through the Wails runtime.
type WailsEmitter struct {
	Ctx context.Context
}

// Emit implements Emitter.
func (w WailsEmitter) Emit(name string, payload any) {
	if w.Ctx == nil {
		return
	}
	runtime.EventsEmit(w.Ctx, name, payload)
}

// NopEmitter discards all events (tests, headless use).
type NopEmitter struct{}

// Emit implements Emitter.
func (NopEmitter) Emit(string, any) {}
