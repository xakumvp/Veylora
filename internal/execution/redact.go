package execution

import (
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Mask is what secrets are replaced with in logs, errors and outputs.
const Mask = "••••••"

var sensitiveKeyParts = []string{
	"authorization", "auth", "token", "secret", "password", "passwd",
	"api-key", "api_key", "apikey", "x-api-key", "cookie", "session", "private",
	"credential", "signature",
}

// IsSensitiveKey reports whether a header / query / field name usually carries a secret.
func IsSensitiveKey(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, p := range sensitiveKeyParts {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

// Redactor remembers secret values seen during a run and masks them in any text.
type Redactor struct {
	mu      sync.RWMutex
	secrets map[string]struct{}
}

// NewRedactor creates an empty redactor.
func NewRedactor() *Redactor {
	return &Redactor{secrets: map[string]struct{}{}}
}

// Add registers a secret value. Very short values are ignored to avoid
// masking unrelated text (e.g. a secret "1" would mask every digit 1).
func (r *Redactor) Add(secret string) {
	s := strings.TrimSpace(secret)
	if len(s) < 4 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secrets[s] = struct{}{}
	// "Bearer abc" — also mask the bare token.
	if _, after, ok := strings.Cut(s, " "); ok && len(after) >= 4 {
		r.secrets[after] = struct{}{}
	}
}

// String masks all known secrets plus credentials embedded in URLs.
func (r *Redactor) String(s string) string {
	if s == "" {
		return s
	}
	r.mu.RLock()
	list := make([]string, 0, len(r.secrets))
	for k := range r.secrets {
		list = append(list, k)
	}
	r.mu.RUnlock()
	// Longest first so that "Bearer xyz" is replaced before "xyz".
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	for _, sec := range list {
		s = strings.ReplaceAll(s, sec, Mask)
	}
	return redactURLsInText(s)
}

// Value walks a JSON-like value and masks secrets in all strings and
// all values stored under sensitive keys.
func (r *Redactor) Value(v any) any {
	switch t := v.(type) {
	case string:
		return r.String(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if IsSensitiveKey(k) {
				if s, ok := val.(string); ok && s != "" {
					out[k] = Mask
					continue
				}
			}
			out[k] = r.Value(val)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if IsSensitiveKey(k) && val != "" {
				out[k] = Mask
			} else {
				out[k] = r.String(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = r.Value(val)
		}
		return out
	default:
		return v
	}
}

// RedactURL strips passwords and sensitive query values from a URL string.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	changed := false
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
			changed = true
		}
	}
	q := u.Query()
	for k := range q {
		if IsSensitiveKey(k) {
			q.Set(k, "xxxxx")
			changed = true
		}
	}
	if !changed {
		return raw
	}
	u.RawQuery = q.Encode()
	return strings.ReplaceAll(u.String(), "xxxxx", Mask)
}

func redactURLsInText(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '"' || r == '\'' || r == '\n' })
	for _, f := range fields {
		if strings.Contains(f, "://") {
			if red := RedactURL(f); red != f {
				s = strings.ReplaceAll(s, f, red)
			}
		}
	}
	return s
}
