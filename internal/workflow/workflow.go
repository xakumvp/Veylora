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
	Position Position       `json:"position"`
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
	SchemaVersion int       `json:"schemaVersion"`
	Nodes         []Node    `json:"nodes"`
	Edges         []Edge    `json:"edges"`
	Viewport      *Viewport `json:"viewport,omitempty"`
}

// New returns an empty document with the current schema version.
func New() Document {
	return Document{SchemaVersion: SchemaVersion, Nodes: []Node{}, Edges: []Edge{}}
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
