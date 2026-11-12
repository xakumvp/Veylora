package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
)

// Fingerprint hashes the meaningful content of a document (viewport excluded),
// so that panning the canvas does not count as a change.
func Fingerprint(d Document) string {
	d.Normalize()
	d.Viewport = nil
	b, _ := json.Marshal(d)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// NodeChange describes a node that exists in both documents but differs.
type NodeChange struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Fields []string `json:"fields"` // name, key, type, params.<key>, settings, position
}

// NodeRef identifies a node in a diff.
type NodeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// DiffResult is a structural comparison of two workflow versions.
type DiffResult struct {
	AddedNodes       []NodeRef    `json:"addedNodes"`
	RemovedNodes     []NodeRef    `json:"removedNodes"`
	ChangedNodes     []NodeChange `json:"changedNodes"`
	AddedEdges       int          `json:"addedEdges"`
	RemovedEdges     int          `json:"removedEdges"`
	VariablesChanged bool         `json:"variablesChanged"`
	SettingsChanged  bool         `json:"settingsChanged"`
	Identical        bool         `json:"identical"`
}

// Diff compares an older document with a newer one.
func Diff(older, newer Document) DiffResult {
	older.Normalize()
	newer.Normalize()
	res := DiffResult{AddedNodes: []NodeRef{}, RemovedNodes: []NodeRef{}, ChangedNodes: []NodeChange{}}

	oldNodes := map[string]Node{}
	for _, n := range older.Nodes {
		oldNodes[n.ID] = n
	}
	newNodes := map[string]Node{}
	for _, n := range newer.Nodes {
		newNodes[n.ID] = n
		o, ok := oldNodes[n.ID]
		if !ok {
			res.AddedNodes = append(res.AddedNodes, NodeRef{ID: n.ID, Name: n.Name, Type: n.Type})
			continue
		}
		if fields := nodeFieldChanges(o, n); len(fields) > 0 {
			res.ChangedNodes = append(res.ChangedNodes, NodeChange{ID: n.ID, Name: n.Name, Fields: fields})
		}
	}
	for _, n := range older.Nodes {
		if _, ok := newNodes[n.ID]; !ok {
			res.RemovedNodes = append(res.RemovedNodes, NodeRef{ID: n.ID, Name: n.Name, Type: n.Type})
		}
	}

	edgeKey := func(e Edge) string { return e.Source + "|" + e.SourceHandle + "|" + e.Target + "|" + e.TargetHandle }
	oldEdges := map[string]bool{}
	for _, e := range older.Edges {
		oldEdges[edgeKey(e)] = true
	}
	newEdges := map[string]bool{}
	for _, e := range newer.Edges {
		newEdges[edgeKey(e)] = true
		if !oldEdges[edgeKey(e)] {
			res.AddedEdges++
		}
	}
	for k := range oldEdges {
		if !newEdges[k] {
			res.RemovedEdges++
		}
	}

	res.VariablesChanged = !reflect.DeepEqual(older.Variables, newer.Variables)
	res.SettingsChanged = older.Settings != newer.Settings
	res.Identical = len(res.AddedNodes) == 0 && len(res.RemovedNodes) == 0 && len(res.ChangedNodes) == 0 &&
		res.AddedEdges == 0 && res.RemovedEdges == 0 && !res.VariablesChanged && !res.SettingsChanged
	return res
}

func nodeFieldChanges(a, b Node) []string {
	var fields []string
	if a.Name != b.Name {
		fields = append(fields, "name")
	}
	if a.Key != b.Key {
		fields = append(fields, "key")
	}
	if a.Type != b.Type {
		fields = append(fields, "type")
	}
	keys := map[string]bool{}
	for k := range a.Params {
		keys[k] = true
	}
	for k := range b.Params {
		keys[k] = true
	}
	var params []string
	for k := range keys {
		if !jsonEqual(a.Params[k], b.Params[k]) {
			params = append(params, "params."+k)
		}
	}
	sort.Strings(params)
	fields = append(fields, params...)
	if !jsonEqual(a.EffectiveSettings(), b.EffectiveSettings()) {
		fields = append(fields, "settings")
	}
	if a.Position != b.Position {
		fields = append(fields, "position")
	}
	return fields
}

// jsonEqual compares values by their JSON form (so 1 == 1.0 after a round trip).
func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
