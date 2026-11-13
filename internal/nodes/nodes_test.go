package nodes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"veylora/internal/execution"
	"veylora/internal/workflow"
)

func input(n workflow.Node, env Env) (*Input, *[]string) {
	var logs []string
	return &Input{
		RunID:    "run-1",
		Node:     n,
		Vars:     execution.NewContext(),
		Redactor: execution.NewRedactor(),
		Env:      env,
		Log: func(level, message string, _ map[string]any) {
			logs = append(logs, level+": "+message)
		},
	}, &logs
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		left  any
		op    string
		right any
		want  bool
	}{
		{float64(200), OpEquals, "200", true},
		{"200", OpEquals, "200.0", true},
		{"abc", OpEquals, "abd", false},
		{"abc", OpNotEquals, "abd", true},
		{float64(5), OpGreaterThan, "3", true},
		{"2.5", OpLessThan, float64(3), true},
		{"hello world", OpContains, "world", true},
		{[]any{"a", float64(1)}, OpContains, "1", true},
		{[]any{"a"}, OpContains, "b", false},
		{"", OpIsEmpty, nil, true},
		{nil, OpIsEmpty, nil, true},
		{[]any{}, OpIsEmpty, nil, true},
		{"x", OpIsEmpty, nil, false},
		{true, OpEquals, "true", true},
	}
	for _, c := range cases {
		got, err := Evaluate(c.left, c.op, c.right)
		if err != nil {
			t.Fatalf("Evaluate(%v %s %v) error: %v", c.left, c.op, c.right, err)
		}
		if got != c.want {
			t.Errorf("Evaluate(%v %s %v) = %v, want %v", c.left, c.op, c.right, got, c.want)
		}
	}
	if _, err := Evaluate("abc", OpGreaterThan, "1"); err == nil {
		t.Error("Greater Than on text must fail")
	}
}

func TestConditionBranch(t *testing.T) {
	in, _ := input(workflow.Node{Params: map[string]any{"variable": "{{http.status}}", "operator": OpEquals, "value": "404"}}, Env{})
	in.Vars.Set("http", map[string]any{"status": float64(200)})
	res, err := Condition{}.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Branch != "false" || res.Output["result"] != false {
		t.Fatalf("want FALSE branch, got %+v", res)
	}
}

func TestDelayCancel(t *testing.T) {
	in, _ := input(workflow.Node{Params: map[string]any{"duration": 10.0, "unit": "s"}}, Env{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := Delay{}.Execute(ctx, in)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("delay was not interrupted")
	}
}

func TestDelayLimits(t *testing.T) {
	if errs := (Delay{}).Validate(map[string]any{"duration": 61.0, "unit": "min"}); len(errs) == 0 {
		t.Fatal("61 minutes must be rejected")
	}
	in, _ := input(workflow.Node{Params: map[string]any{"duration": 5.0, "unit": "ms"}}, Env{})
	if _, err := (Delay{}).Execute(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}

func TestLoggerResolvesVariables(t *testing.T) {
	in, logs := input(workflow.Node{Params: map[string]any{
		"message": "status {{http.status}}", "level": "warning",
		"variables": []any{map[string]any{"key": "code", "value": "{{http.status}}", "enabled": true}},
	}}, Env{})
	in.Vars.Set("http", map[string]any{"status": float64(201)})
	res, err := Logger{}.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(*logs) != 1 || (*logs)[0] != "WARNING: status 201" {
		t.Fatalf("unexpected logs %v", *logs)
	}
	if res.Output["variables"].(map[string]any)["code"] != float64(201) {
		t.Fatalf("variables not resolved: %+v", res.Output)
	}
}

func TestHTTPRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-value" || r.URL.Query().Get("q") != "go" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"OK","n":3}`))
	}))
	defer srv.Close()

	in, _ := input(workflow.Node{Params: map[string]any{
		"method": "GET", "url": srv.URL, "timeout": 5.0,
		"headers": []any{map[string]any{"key": "Authorization", "value": "Bearer secret-value", "enabled": true}},
		"query":   []any{map[string]any{"key": "q", "value": "go"}},
	}}, Env{AllowLocalNetwork: true})
	res, err := HTTPRequest{}.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output["status"] != float64(200) {
		t.Fatalf("status %v", res.Output["status"])
	}
	if res.Output["body"].(map[string]any)["message"] != "OK" {
		t.Fatalf("body %v", res.Output["body"])
	}
	if strings.Contains(in.Redactor.String("Bearer secret-value"), "secret-value") {
		t.Fatal("Authorization header was not registered as secret")
	}
}

func TestHTTPBlocksLocalNetworkByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	in, _ := input(workflow.Node{Params: map[string]any{"method": "GET", "url": srv.URL, "timeout": 5.0}}, Env{})
	_, err := HTTPRequest{}.Execute(context.Background(), in)
	if !errors.Is(err, ErrLocalNetworkBlocked) {
		t.Fatalf("want ErrLocalNetworkBlocked, got %v", err)
	}
}

func TestHTTPResponseLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
	}))
	defer srv.Close()
	in, _ := input(workflow.Node{Params: map[string]any{"method": "GET", "url": srv.URL, "timeout": 5.0}},
		Env{AllowLocalNetwork: true, MaxResponseBytes: 1024})
	if _, err := (HTTPRequest{}).Execute(context.Background(), in); err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestHTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(3 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	in, _ := input(workflow.Node{Params: map[string]any{"method": "GET", "url": srv.URL, "timeout": 1.0}}, Env{AllowLocalNetwork: true})
	_, err := HTTPRequest{}.Execute(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "время ожидания") {
		t.Fatalf("want timeout error, got %v", err)
	}
}

func TestHTTPRejectsNonHTTPSchemes(t *testing.T) {
	for _, u := range []string{"file:///etc/passwd", "ftp://example.com", "C:\\Windows\\win.ini", "//example.com"} {
		if ValidateURL(u) == nil {
			t.Errorf("%s must be rejected", u)
		}
	}
}

func TestRegistryDefinitions(t *testing.T) {
	defs := NewRegistry().Definitions()
	if len(defs) != 25 {
		t.Fatalf("want 25 node types, got %d", len(defs))
	}
	if defs[0].Type != TypeManualTrigger {
		t.Fatalf("Triggers must come first, got %s", defs[0].Type)
	}
}
