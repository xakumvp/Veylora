package execution

import (
	"strings"
	"testing"
)

func newCtx() *Context {
	c := NewContext()
	c.Set("http", map[string]any{
		"status": float64(200),
		"body": map[string]any{
			"message": "OK",
			"items":   []any{map[string]any{"name": "first"}, map[string]any{"name": "second"}},
		},
		"headers": map[string]string{"Content-Type": "application/json"},
	})
	c.Set("trigger", map[string]any{"runId": "r-1"})
	return c
}

func TestResolveString(t *testing.T) {
	c := newCtx()
	cases := map[string]string{
		"status={{http.status}}":            "status=200",
		"{{ http.body.message }}":           "OK",
		"{{http.body.items.1.name}}":        "second",
		"{{http.headers.Content-Type}}":     "application/json",
		"missing: [{{nope.value}}]":         "missing: []",
		"no templates":                      "no templates",
		"{{http.body.items.9.name}}":        "",
		"{{trigger.runId}}/{{http.status}}": "r-1/200",
	}
	for in, want := range cases {
		if got := c.ResolveString(in); got != want {
			t.Errorf("ResolveString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveValueKeepsType(t *testing.T) {
	c := newCtx()
	if v := c.ResolveValue("{{http.status}}"); v != float64(200) {
		t.Fatalf("want float64 200, got %#v", v)
	}
	if v := c.ResolveValue(" {{http.body.items}} "); v == nil {
		t.Fatal("want slice, got nil")
	}
	if v := c.ResolveValue("code {{http.status}}"); v != "code 200" {
		t.Fatalf("mixed template should be string, got %#v", v)
	}
	if v := c.ResolveValue("{{missing}}"); v != nil {
		t.Fatalf("missing var should be nil, got %#v", v)
	}
}

func TestStringifyObject(t *testing.T) {
	if got := Stringify(map[string]any{"a": 1.0}); got != `{"a":1}` {
		t.Fatalf("got %s", got)
	}
}

func TestRedactor(t *testing.T) {
	r := NewRedactor()
	r.Add("Bearer super-secret-token")
	r.Add("abc") // too short, ignored
	msg := r.String("header Bearer super-secret-token and super-secret-token, abc")
	if strings.Contains(msg, "super-secret-token") {
		t.Fatalf("secret leaked: %s", msg)
	}
	if !strings.Contains(msg, "abc") {
		t.Fatalf("short value should not be masked: %s", msg)
	}

	v := r.Value(map[string]any{
		"Authorization": "anything",
		"nested":        []any{"x super-secret-token y"},
		"plain":         "hello",
	}).(map[string]any)
	if v["Authorization"] != Mask || v["plain"] != "hello" {
		t.Fatalf("unexpected redaction: %#v", v)
	}
	if strings.Contains(v["nested"].([]any)[0].(string), "super-secret") {
		t.Fatal("nested secret leaked")
	}
}

func TestRedactURL(t *testing.T) {
	got := RedactURL("https://user:pass@example.com/x?api_key=123456&page=2")
	if strings.Contains(got, "pass") || strings.Contains(got, "123456") || !strings.Contains(got, "page=2") {
		t.Fatalf("bad redaction: %s", got)
	}
	r := NewRedactor()
	text := r.String(`Get "https://example.com/?token=abcdef": EOF`)
	if strings.Contains(text, "abcdef") {
		t.Fatalf("token in error leaked: %s", text)
	}
}

func TestIsSensitiveKey(t *testing.T) {
	for _, k := range []string{"Authorization", "X-API-Key", "access_token", "Cookie", "client_secret"} {
		if !IsSensitiveKey(k) {
			t.Errorf("%s should be sensitive", k)
		}
	}
	for _, k := range []string{"Content-Type", "Accept", "page"} {
		if IsSensitiveKey(k) {
			t.Errorf("%s should not be sensitive", k)
		}
	}
}
