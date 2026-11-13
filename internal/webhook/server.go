package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Default limits for the webhook server.
const (
	DefaultPort        = 5678
	DefaultHost        = "127.0.0.1"
	DefaultMaxBodyMB   = 5
	DefaultRateLimit   = 120 // requests per minute per IP
	ServerShutdownWait = 3 * time.Second
)

// Request is the structured incoming payload passed to the workflow engine.
type Request struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	Query    map[string]string `json:"query"`
	Body     any               `json:"body"`
	RawBody  string            `json:"rawBody"`
	RemoteIP string            `json:"ip"`
}

// Registration describes one armed webhook trigger.
type Registration struct {
	WorkflowID   string `json:"workflowId"`
	NodeID       string `json:"nodeId"`
	Endpoint     string `json:"endpoint"`
	Method       string `json:"method"`   // ANY, GET, POST, PUT, DELETE
	AuthMode     string `json:"authMode"` // none, token, header
	Secret       string `json:"secret"`
	HeaderName   string `json:"headerName"` // e.g. X-Webhook-Secret
	ResponseCode int    `json:"responseCode"`
	ResponseBody string `json:"responseBody"`
}

// TriggerHandler is called when an authorized webhook request matches an active endpoint.
type TriggerHandler func(workflowID, nodeID string, req Request) (runID string, err error)

// Server is the embedded HTTP server receiving external webhook events.
type Server struct {
	mu           sync.RWMutex
	logger       *slog.Logger
	host         string
	port         int
	maxBodyBytes int64
	rateLimit    int

	endpoints   map[string]Registration // lowercase endpoint -> registration
	byWorkflow  map[string][]string     // workflowID -> list of endpoints
	limiter     *ipRateLimiter
	onTrigger   TriggerHandler
	httpServer  *http.Server
	listener    net.Listener
	running     bool
}

// New creates an unstarted webhook server.
func New(logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		logger:       logger,
		host:         DefaultHost,
		port:         DefaultPort,
		maxBodyBytes: DefaultMaxBodyMB << 20,
		rateLimit:    DefaultRateLimit,
		endpoints:    make(map[string]Registration),
		byWorkflow:   make(map[string][]string),
		limiter:      newRateLimiter(DefaultRateLimit),
	}
}

// SetTriggerHandler sets the callback that launches workflows.
func (s *Server) SetTriggerHandler(fn TriggerHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTrigger = fn
}

// Register registers an active webhook endpoint.
func (s *Server) Register(reg Registration) error {
	ep := cleanEndpoint(reg.Endpoint)
	if ep == "" {
		return errors.New("пустой endpoint для вебхука")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check conflict with another workflow
	if existing, found := s.endpoints[ep]; found {
		if existing.WorkflowID != reg.WorkflowID || existing.NodeID != reg.NodeID {
			return fmt.Errorf("эндпоинт %q уже используется другим сценарием", ep)
		}
	}

	reg.Endpoint = ep
	if reg.Method == "" {
		reg.Method = "POST"
	} else {
		reg.Method = strings.ToUpper(reg.Method)
	}
	if reg.ResponseCode <= 0 {
		reg.ResponseCode = 200
	}
	if reg.ResponseBody == "" {
		reg.ResponseBody = `{"status":"ok"}`
	}
	if reg.HeaderName == "" {
		reg.HeaderName = "X-Webhook-Secret"
	}

	s.endpoints[ep] = reg
	s.byWorkflow[reg.WorkflowID] = append(s.byWorkflow[reg.WorkflowID], ep)
	s.logger.Info("webhook registered", "endpoint", ep, "workflow", reg.WorkflowID, "node", reg.NodeID)
	return nil
}

// Unregister removes a single endpoint.
func (s *Server) Unregister(endpoint string) {
	ep := cleanEndpoint(endpoint)
	s.mu.Lock()
	defer s.mu.Unlock()

	if reg, ok := s.endpoints[ep]; ok {
		delete(s.endpoints, ep)
		// Clean from byWorkflow
		list := s.byWorkflow[reg.WorkflowID]
		filtered := make([]string, 0, len(list))
		for _, e := range list {
			if e != ep {
				filtered = append(filtered, e)
			}
		}
		s.byWorkflow[reg.WorkflowID] = filtered
		s.logger.Info("webhook unregistered", "endpoint", ep, "workflow", reg.WorkflowID)
	}
}

// UnregisterWorkflow removes all endpoints associated with a workflow.
func (s *Server) UnregisterWorkflow(workflowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	endpoints := s.byWorkflow[workflowID]
	for _, ep := range endpoints {
		delete(s.endpoints, ep)
	}
	delete(s.byWorkflow, workflowID)
	if len(endpoints) > 0 {
		s.logger.Info("workflow webhooks removed", "workflow", workflowID, "count", len(endpoints))
	}
}

// Endpoints returns all active registrations.
func (s *Server) Endpoints() []Registration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Registration, 0, len(s.endpoints))
	for _, reg := range s.endpoints {
		out = append(out, reg)
	}
	return out
}

// IsRunning reports whether the webhook server is listening.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Addr returns the listening network address, or "" if not running.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// Start begins listening on the configured host and port.
func (s *Server) Start(host string, port int) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	if host == "" {
		host = DefaultHost
	}
	if port <= 0 {
		port = DefaultPort
	}
	s.host = host
	s.port = port

	addr := fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("не удалось запустить Webhook Server на %s: %w", addr, err)
	}

	s.listener = ln
	s.running = true

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHTTP)

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	s.mu.Unlock()

	s.logger.Info("webhook server started", "addr", addr)

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("webhook server serve error", "error", err)
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running || s.httpServer == nil {
		s.mu.Unlock()
		return nil
	}
	srv := s.httpServer
	s.running = false
	s.mu.Unlock()

	s.logger.Info("stopping webhook server")
	err := srv.Shutdown(ctx)
	s.mu.Lock()
	s.httpServer = nil
	s.listener = nil
	s.mu.Unlock()
	return err
}

// cleanEndpoint strips leading/trailing slashes and lowercases.
func cleanEndpoint(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	// Strip prefix if user passed "/webhook/test"
	if strings.HasPrefix(path, "webhook/") {
		path = strings.TrimPrefix(path, "webhook/")
	}
	if strings.HasPrefix(path, "api/v1/webhook/") {
		path = strings.TrimPrefix(path, "api/v1/webhook/")
	}
	return strings.ToLower(strings.Trim(path, "/"))
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Health check
	if r.URL.Path == "/" || r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"server": "veylora-webhook",
		})
		return
	}

	// 2. Extract endpoint
	path := cleanEndpoint(r.URL.Path)
	if path == "" {
		http.Error(w, `{"error":"endpoint not specified"}`, http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	reg, found := s.endpoints[path]
	onTrig := s.onTrigger
	maxBytes := s.maxBodyBytes
	s.mu.RUnlock()

	if !found {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":    "webhook endpoint not found",
			"endpoint": path,
		})
		return
	}

	// 3. Check Method
	if reg.Method != "ANY" && !strings.EqualFold(reg.Method, r.Method) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Allow", reg.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":    fmt.Sprintf("method %s not allowed, expected %s", r.Method, reg.Method),
			"expected": reg.Method,
		})
		return
	}

	// 4. Rate Limiting
	ip := clientIP(r)
	if !s.limiter.allow(ip) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "rate limit exceeded, please retry later",
		})
		return
	}

	// 5. Authentication check
	if !s.checkAuth(reg, r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "unauthorized webhook request",
		})
		return
	}

	// 6. Read Request Body
	if r.ContentLength > maxBytes {
		http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
		return
	}
	bodyData, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	if int64(len(bodyData)) > maxBytes {
		http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	rawBody := string(bodyData)
	var parsedBody any
	if len(bodyData) > 0 {
		if json.Valid(bodyData) {
			_ = json.Unmarshal(bodyData, &parsedBody)
		} else {
			parsedBody = rawBody
		}
	}

	// Headers map
	reqHeaders := make(map[string]string)
	for k, v := range r.Header {
		reqHeaders[strings.ToLower(k)] = strings.Join(v, ", ")
	}

	// Query map
	reqQuery := make(map[string]string)
	for k, v := range r.URL.Query() {
		reqQuery[k] = strings.Join(v, ", ")
	}

	reqPayload := Request{
		Endpoint: path,
		Method:   r.Method,
		Headers:  reqHeaders,
		Query:    reqQuery,
		Body:     parsedBody,
		RawBody:  rawBody,
		RemoteIP: ip,
	}

	// 7. Fire trigger
	var runID string
	if onTrig != nil {
		var trigErr error
		runID, trigErr = onTrig(reg.WorkflowID, reg.NodeID, reqPayload)
		if trigErr != nil {
			s.logger.Warn("webhook trigger launch error", "workflow", reg.WorkflowID, "error", trigErr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   trigErr.Error(),
				"status":  "rejected",
			})
			return
		}
	}

	// 8. Respond to caller
	code := reg.ResponseCode
	if code < 200 || code > 299 {
		code = http.StatusOK
	}

	bodyTemplate := reg.ResponseBody
	if bodyTemplate == "" {
		bodyTemplate = `{"status":"ok"}`
	}
	respBody := strings.ReplaceAll(bodyTemplate, "{{runId}}", runID)
	respBody = strings.ReplaceAll(respBody, "{runId}", runID)

	if json.Valid([]byte(respBody)) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(code)
	_, _ = w.Write([]byte(respBody))
}

func (s *Server) checkAuth(reg Registration, r *http.Request) bool {
	switch strings.ToLower(reg.AuthMode) {
	case "", "none":
		return true
	case "token":
		// Query param ?token=... or ?secret=... or Authorization: Bearer <secret>
		secret := strings.TrimSpace(reg.Secret)
		if secret == "" {
			return true
		}
		if qToken := r.URL.Query().Get("token"); qToken == secret {
			return true
		}
		if qSecret := r.URL.Query().Get("secret"); qSecret == secret {
			return true
		}
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			token := strings.TrimSpace(authHeader[7:])
			if token == secret {
				return true
			}
		}
		return false
	case "header":
		secret := strings.TrimSpace(reg.Secret)
		if secret == "" {
			return true
		}
		headerName := reg.HeaderName
		if headerName == "" {
			headerName = "X-Webhook-Secret"
		}
		val := strings.TrimSpace(r.Header.Get(headerName))
		return val == secret
	default:
		return true
	}
}

// ---------- In-Memory IP Rate Limiter ----------

type ipRateLimiter struct {
	mu      sync.Mutex
	limit   int
	history map[string][]int64 // ip -> unix timestamps in seconds
}

func newRateLimiter(limit int) *ipRateLimiter {
	return &ipRateLimiter{
		limit:   limit,
		history: make(map[string][]int64),
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	if l.limit <= 0 {
		return true
	}
	now := time.Now().Unix()
	windowStart := now - 60

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamps := l.history[ip]
	valid := make([]int64, 0, len(timestamps))
	for _, ts := range timestamps {
		if ts > windowStart {
			valid = append(valid, ts)
		}
	}

	if len(valid) >= l.limit {
		l.history[ip] = valid
		return false
	}

	valid = append(valid, now)
	l.history[ip] = valid
	return true
}
