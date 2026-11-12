// Package workflow describes the persisted shape of a Veylora workflow:
// nodes, connections between their ports and canvas metadata.
package workflow

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion is the current version of the workflow document format.
// Bump it whenever the JSON layout changes in a non-backward-compatible way.
const SchemaVersion = 1

// Position is a node's location on the canvas.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Error handling modes of a node.
const (
	OnErrorStop        = "stop"         // fail the node and stop this path (default)
	OnErrorContinue    = "continue"     // treat the failure as success and continue via the normal output
	OnErrorErrorOutput = "error_output" // continue via the node's "error" output port
)

// Retry configures repeated attempts of a failing node.
type Retry struct {
	MaxAttempts int  `json:"maxAttempts"` // total attempts including the first; 0/1 = no retry
	DelayMs     int  `json:"delayMs"`
	Backoff     bool `json:"backoff"` // exponential: delay * 2^(attempt-1)
	MaxDelayMs  int  `json:"maxDelayMs"`
}

// NodeSettings are execution options shared by every node type.
type NodeSettings struct {
	Retry      Retry   `json:"retry"`
	TimeoutSec float64 `json:"timeoutSec"` // 0 = no node-level timeout
	OnError    string  `json:"onError"`
}

// Node is a single functional block on the canvas.
//
// Key is the variable name under which the node's output is published in the
// execution context, e.g. a node with key "http" is referenced as {{http.status}}.
type Node struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Key      string         `json:"key"`
	Params   map[string]any `json:"params"`
	Settings *NodeSettings  `json:"settings,omitempty"`
	Position Position       `json:"position"`
}

// EffectiveSettings returns the node settings with defaults applied.
func (n Node) EffectiveSettings() NodeSettings {
	s := NodeSettings{OnError: OnErrorStop}
	if n.Settings != nil {
		s = *n.Settings
	}
	if s.OnError == "" {
		s.OnError = OnErrorStop
	}
	if s.Retry.MaxAttempts < 1 {
		s.Retry.MaxAttempts = 1
	}
	return s
}

// Variable is a workflow-level constant available as {{variables.<key>}}.
type Variable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Settings are workflow-level execution options.
type Settings struct {
	TimeoutSec float64 `json:"timeoutSec"` // 0 = no workflow timeout
}

// Edge connects an output port of one node to an input port of another.
type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceHandle string `json:"sourceHandle"`
	Target       string `json:"target"`
	TargetHandle string `json:"targetHandle"`
}

// Viewport is the saved canvas pan/zoom.
type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// Document is the full serialisable workflow graph.
type Document struct {
	SchemaVersion int        `json:"schemaVersion"`
	Nodes         []Node     `json:"nodes"`
	Edges         []Edge     `json:"edges"`
	Variables     []Variable `json:"variables"`
	Settings      Settings   `json:"settings"`
	Viewport      *Viewport  `json:"viewport,omitempty"`
}

// New returns an empty document with the current schema version.
func New() Document {
	return Document{SchemaVersion: SchemaVersion, Nodes: []Node{}, Edges: []Edge{}, Variables: []Variable{}}
}

// Normalize fills nil slices/maps so the document always serialises predictably.
func (d *Document) Normalize() {
	if d.SchemaVersion == 0 {
		d.SchemaVersion = SchemaVersion
	}
	if d.Nodes == nil {
		d.Nodes = []Node{}
	}
	if d.Edges == nil {
		d.Edges = []Edge{}
	}
	if d.Variables == nil {
		d.Variables = []Variable{}
	}
	for i := range d.Nodes {
		if d.Nodes[i].Params == nil {
			d.Nodes[i].Params = map[string]any{}
		}
	}
}

// NodeByID returns the node with the given ID.
func (d *Document) NodeByID(id string) (*Node, bool) {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i], true
		}
	}
	return nil, false
}

// Parse decodes a workflow document from JSON.
func Parse(data []byte) (Document, error) {
	var d Document
	if err := json.Unmarshal(data, &d); err != nil {
		return Document{}, fmt.Errorf("некорректный JSON workflow: %w", err)
	}
	d.Normalize()
	return d, nil
}

// Marshal encodes a workflow document as compact JSON.
func Marshal(d Document) ([]byte, error) {
	d.Normalize()
	return json.Marshal(d)
}
