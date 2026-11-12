// Package execution holds the per-run state shared between nodes:
// the variable context, template resolution and secret redaction.
package execution

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Context is the single variable scope of one workflow run.
// Every node publishes its output under its key; later nodes read it via {{key.path}}.
type Context struct {
	mu   sync.RWMutex
	vars map[string]any
}

// NewContext creates an empty variable context.
func NewContext() *Context {
	return &Context{vars: map[string]any{}}
}

// Set publishes a value under a top-level key.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vars[key] = value
}

// Snapshot returns a shallow copy of all variables.
func (c *Context) Snapshot() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]any, len(c.vars))
	for k, v := range c.vars {
		out[k] = v
	}
	return out
}

// Lookup resolves a dotted path such as "http.body.items.0.name".
func (c *Context) Lookup(path string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, false
	}
	cur, ok := c.vars[parts[0]]
	if !ok {
		return nil, false
	}
	for _, p := range parts[1:] {
		switch v := cur.(type) {
		case map[string]any:
			cur, ok = v[p]
			if !ok {
				return nil, false
			}
		case map[string]string:
			s, ok := v[p]
			if !ok {
				return nil, false
			}
			cur = s
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

var templateRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_\-]+)*)\s*\}\}`)

// ResolveString substitutes every {{path}} in s with the textual form of its value.
// Unknown variables become an empty string.
func (c *Context) ResolveString(s string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return templateRe.ReplaceAllStringFunc(s, func(m string) string {
		path := templateRe.FindStringSubmatch(m)[1]
		v, ok := c.Lookup(path)
		if !ok {
			return ""
		}
		return Stringify(v)
	})
}

// ResolveValue behaves like ResolveString, except that a string consisting of
// exactly one template returns the raw (typed) value, so numbers stay numbers.
func (c *Context) ResolveValue(s string) any {
	trimmed := strings.TrimSpace(s)
	if loc := templateRe.FindStringSubmatchIndex(trimmed); loc != nil && loc[0] == 0 && loc[1] == len(trimmed) {
		v, ok := c.Lookup(trimmed[loc[2]:loc[3]])
		if !ok {
			return nil
		}
		return v
	}
	return c.ResolveString(s)
}

// Stringify renders any value as text: strings as-is, everything else as JSON.
func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}
