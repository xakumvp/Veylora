package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer() *Server {
	s := New(nil)
	return s
}

func TestServerHealth(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	s.handleHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["status"] != "ok" {
		t.Fatalf("unexpected health response: %+v", res)
	}
}

func TestServerWebhookExecution(t *testing.T) {
	s := testServer()
	var triggeredReq Request
	var triggeredWf string

	s.SetTriggerHandler(func(workflowID, nodeID string, req Request) (string, error) {
		triggeredWf = workflowID
		triggeredReq = req
		return "run-12345", nil
	})

	err := s.Register(Registration{
		WorkflowID:   "wf-1",
		NodeID:       "node-webhook",
		Endpoint:     "github-event",
		Method:       "POST",
		AuthMode:     "none",
		ResponseCode: 202,
		ResponseBody: `{"status":"accepted","run":"{{runId}}"}`,
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Send POST /webhook/github-event
	payload := `{"action":"opened","issue":{"number":42}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/github-event?ref=main", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleHTTP(rec, req)

	if rec.Code != 202 {
		t.Fatalf("want status 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if triggeredWf != "wf-1" {
		t.Fatalf("want workflow wf-1, got %s", triggeredWf)
	}
	if triggeredReq.Method != "POST" || triggeredReq.Endpoint != "github-event" {
		t.Fatalf("unexpected request: %+v", triggeredReq)
	}
	if triggeredReq.Query["ref"] != "main" {
		t.Fatalf("want query ref=main, got %s", triggeredReq.Query["ref"])
	}
	bodyMap, ok := triggeredReq.Body.(map[string]any)
	if !ok || bodyMap["action"] != "opened" {
		t.Fatalf("want body.action=opened, got %+v", triggeredReq.Body)
	}
	if !strings.Contains(rec.Body.String(), "run-12345") {
		t.Fatalf("want run-12345 in response, got %s", rec.Body.String())
	}
}

func TestServerMethodNotAllowed(t *testing.T) {
	s := testServer()
	_ = s.Register(Registration{
		WorkflowID: "wf-1",
		NodeID:     "n1",
		Endpoint:   "post-only",
		Method:     "POST",
	})

	req := httptest.NewRequest(http.MethodGet, "/webhook/post-only", nil)
	rec := httptest.NewRecorder()

	s.handleHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func TestServerAuthentication(t *testing.T) {
	s := testServer()
	_ = s.Register(Registration{
		WorkflowID: "wf-1",
		NodeID:     "n1",
		Endpoint:   "token-auth",
		Method:     "POST",
		AuthMode:   "token",
		Secret:     "secret-token-123",
	})

	_ = s.Register(Registration{
		WorkflowID: "wf-1",
		NodeID:     "n2",
		Endpoint:   "header-auth",
		Method:     "POST",
		AuthMode:   "header",
		HeaderName: "X-Custom-Secret",
		Secret:     "custom-secret-456",
	})

	// 1. Without auth -> 401
	req := httptest.NewRequest(http.MethodPost, "/webhook/token-auth", nil)
	rec := httptest.NewRecorder()
	s.handleHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}

	// 2. Bearer token -> 200
	req = httptest.NewRequest(http.MethodPost, "/webhook/token-auth", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	rec = httptest.NewRecorder()
	s.handleHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with Bearer, got %d", rec.Code)
	}

	// 3. Query param token -> 200
	req = httptest.NewRequest(http.MethodPost, "/webhook/token-auth?token=secret-token-123", nil)
	rec = httptest.NewRecorder()
	s.handleHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with query token, got %d", rec.Code)
	}

	// 4. Header auth -> 200
	req = httptest.NewRequest(http.MethodPost, "/webhook/header-auth", nil)
	req.Header.Set("X-Custom-Secret", "custom-secret-456")
	rec = httptest.NewRecorder()
	s.handleHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with header auth, got %d", rec.Code)
	}
}

func TestServerRateLimit(t *testing.T) {
	s := testServer()
	s.limiter = newRateLimiter(3) // limit to 3 requests per minute

	_ = s.Register(Registration{
		WorkflowID: "wf-1",
		NodeID:     "n1",
		Endpoint:   "limited",
		Method:     "GET",
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/webhook/limited", nil)
		rec := httptest.NewRecorder()
		s.handleHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d want 200, got %d", i, rec.Code)
		}
	}

	// 4th request must be rejected with 429
	req := httptest.NewRequest(http.MethodGet, "/webhook/limited", nil)
	rec := httptest.NewRecorder()
	s.handleHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 Too Many Requests, got %d", rec.Code)
	}
}

func TestServerLifecycle(t *testing.T) {
	s := testServer()
	// Pick random local port
	if err := s.Start("127.0.0.1", 0); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer s.Stop(context.Background())

	if !s.IsRunning() || s.Addr() == "" {
		t.Fatalf("server not running properly: running=%v, addr=%s", s.IsRunning(), s.Addr())
	}

	addr := s.Addr()
	res, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if !bytes.Contains(body, []byte("veylora-webhook")) {
		t.Fatalf("unexpected body: %s", string(body))
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if s.IsRunning() {
		t.Fatalf("expected server to be stopped")
	}
}
