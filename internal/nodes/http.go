package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"veylora/internal/execution"
)

// HTTP limits.
const (
	DefaultHTTPTimeout      = 30 * time.Second
	MaxHTTPTimeout          = 120 * time.Second
	DefaultMaxResponseBytes = 5 << 20 // 5 MB
	MaxRequestBodyBytes     = 5 << 20
	maxRedirects            = 5
)

var httpMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// ErrLocalNetworkBlocked is returned when a request targets localhost or a private network.
var ErrLocalNetworkBlocked = errors.New("доступ к локальной сети и localhost запрещён (можно разрешить в Settings)")

// HTTPRequest performs an HTTP call with strict time and size limits.
type HTTPRequest struct{}

func (HTTPRequest) Definition() Definition {
	methods := make([]Option, len(httpMethods))
	for i, m := range httpMethods {
		methods[i] = Option{Value: m, Label: m}
	}
	return Definition{
		Type:        TypeHTTPRequest,
		Name:        "HTTP Request",
		Category:    "HTTP",
		Description: "Выполняет HTTP-запрос и возвращает статус, заголовки и тело ответа.",
		Icon:        "globe",
		KeyPrefix:   "http",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "method", Label: "Метод", Kind: ParamSelect, Required: true, Default: "GET", Options: methods},
			{Key: "url", Label: "URL", Kind: ParamString, Required: true, Templatable: true, Placeholder: "https://api.example.com/items"},
			{Key: "query", Label: "Query Parameters", Kind: ParamKeyValue, Templatable: true},
			{Key: "headers", Label: "Headers", Kind: ParamKeyValue, Templatable: true,
				Description: "Значения Authorization, токенов и ключей маскируются в логах и не попадают в экспорт."},
			{Key: "body", Label: "Request Body", Kind: ParamText, Templatable: true, Placeholder: `{"name": "{{trigger.runName}}"}`,
				OptionalWhen: &ParamCondition{Param: "method", In: []string{"GET", "DELETE"}}},
			{Key: "timeout", Label: "Timeout, сек", Kind: ParamNumber, Required: true, Default: 30, Min: ptr(1), Max: ptr(MaxHTTPTimeout.Seconds())},
			{Key: "failOnError", Label: "Ошибка при статусе 4xx/5xx", Kind: ParamBoolean, Default: false},
		},
		OutputDoc: []Field{
			{Name: "status", Description: "HTTP Status Code"},
			{Name: "statusText", Description: "Текст статуса"},
			{Name: "headers", Description: "Response Headers"},
			{Name: "body", Description: "Тело ответа (JSON разбирается автоматически)"},
			{Name: "timeMs", Description: "Время ответа, мс"},
			{Name: "size", Description: "Размер ответа, байт"},
		},
	}
}

// ValidateURL checks that a URL is an absolute http(s) URL.
func ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("некорректный URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("поддерживаются только http:// и https:// адреса")
	}
	if u.Host == "" {
		return fmt.Errorf("в URL не указан хост")
	}
	return nil
}

func (HTTPRequest) Validate(params map[string]any) []FieldError {
	var errs []FieldError
	raw := Str(params, "url", "")
	if raw != "" && !HasTemplate(raw) {
		if err := ValidateURL(raw); err != nil {
			errs = append(errs, FieldError{Field: "url", Message: err.Error()})
		}
	}
	m := strings.ToUpper(Str(params, "method", "GET"))
	valid := false
	for _, hm := range httpMethods {
		if m == hm {
			valid = true
		}
	}
	if !valid {
		errs = append(errs, FieldError{Field: "method", Message: "неподдерживаемый метод"})
	}
	for _, kv := range KeyValues(params, "headers") {
		if strings.ContainsAny(kv.Key, " \r\n:") {
			errs = append(errs, FieldError{Field: "headers", Message: fmt.Sprintf("некорректное имя заголовка %q", kv.Key)})
		}
	}
	return errs
}

// isLocalIP reports addresses that point to this machine or a private network.
func isLocalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		// 100.64.0.0/10 carrier-grade NAT and 169.254.169.254-style metadata are covered above/here.
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xC0 == 64)
}

// newHTTPClient builds a client whose dialer re-checks every resolved IP,
// which also protects against DNS rebinding and redirects to local addresses.
func newHTTPClient(env Env, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if env.AllowLocalNetwork {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || isLocalIP(ip) {
				return ErrLocalNetworkBlocked
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		DisableCompression:    false,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("слишком много перенаправлений (больше %d)", maxRedirects)
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("перенаправление на неподдерживаемую схему %q", req.URL.Scheme)
			}
			return nil
		},
	}
}

func (HTTPRequest) Execute(ctx context.Context, in *Input) (Result, error) {
	p := in.Node.Params
	method := strings.ToUpper(Str(p, "method", "GET"))

	// Register secrets before anything can be logged.
	headers := KeyValues(p, "headers")
	query := KeyValues(p, "query")
	resolvedHeaders := make([]KV, 0, len(headers))
	for _, h := range headers {
		if !h.Enabled {
			continue
		}
		v := in.Vars.ResolveString(h.Value)
		if execution.IsSensitiveKey(h.Key) {
			in.Redactor.Add(v)
		}
		resolvedHeaders = append(resolvedHeaders, KV{Key: h.Key, Value: v, Enabled: true})
	}

	rawURL := strings.TrimSpace(in.Vars.ResolveString(Str(p, "url", "")))
	if err := ValidateURL(rawURL); err != nil {
		return Result{}, err
	}
	u, _ := url.Parse(rawURL)
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			in.Redactor.Add(pw)
		}
	}
	q := u.Query()
	for _, kv := range query {
		if !kv.Enabled {
			continue
		}
		v := in.Vars.ResolveString(kv.Value)
		if execution.IsSensitiveKey(kv.Key) {
			in.Redactor.Add(v)
		}
		q.Add(kv.Key, v)
	}
	u.RawQuery = q.Encode()

	timeout := DefaultHTTPTimeout
	if n, ok := Num(p, "timeout"); ok && n > 0 {
		timeout = time.Duration(n * float64(time.Second))
	}
	if timeout > MaxHTTPTimeout {
		timeout = MaxHTTPTimeout
	}

	var body io.Reader
	bodyStr := ""
	if method != "GET" {
		bodyStr = in.Vars.ResolveString(Str(p, "body", ""))
		if len(bodyStr) > MaxRequestBodyBytes {
			return Result{}, fmt.Errorf("тело запроса превышает %d МБ", MaxRequestBodyBytes>>20)
		}
		if bodyStr != "" {
			body = strings.NewReader(bodyStr)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return Result{}, fmt.Errorf("не удалось создать запрос: %w", err)
	}
	req.Header.Set("User-Agent", "Veylora/0.1")
	for _, h := range resolvedHeaders {
		req.Header.Add(h.Key, h.Value)
	}
	if bodyStr != "" && req.Header.Get("Content-Type") == "" {
		if json.Valid([]byte(bodyStr)) {
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	}

	in.Log(LevelDebug, fmt.Sprintf("%s %s", method, execution.RedactURL(u.String())), nil)

	maxBytes := in.Env.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	}

	start := time.Now()
	resp, err := newHTTPClient(in.Env, timeout).Do(req)
	if err != nil {
		return Result{}, describeHTTPError(ctx, err, timeout)
	}
	defer resp.Body.Close()

	if resp.ContentLength > maxBytes {
		return Result{}, fmt.Errorf("ответ слишком большой: %d байт (лимит %d МБ)", resp.ContentLength, maxBytes>>20)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return Result{}, describeHTTPError(ctx, err, timeout)
	}
	if int64(len(data)) > maxBytes {
		return Result{}, fmt.Errorf("ответ превышает лимит %d МБ", maxBytes>>20)
	}
	elapsed := time.Since(start)

	respHeaders := map[string]any{}
	for k, v := range resp.Header {
		respHeaders[k] = strings.Join(v, ", ")
	}

	out := map[string]any{
		"status":     resp.StatusCode,
		"statusText": http.StatusText(resp.StatusCode),
		"headers":    respHeaders,
		"body":       decodeBody(data, resp.Header.Get("Content-Type")),
		"timeMs":     elapsed.Milliseconds(),
		"size":       len(data),
	}
	// Store numbers as float64 like JSON-decoded values, so comparisons behave uniformly.
	out["status"] = float64(resp.StatusCode)
	out["timeMs"] = float64(elapsed.Milliseconds())
	out["size"] = float64(len(data))

	if Bool(p, "failOnError", false) && resp.StatusCode >= 400 {
		return Result{Output: out}, fmt.Errorf("сервер вернул статус %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return Result{Output: out}, nil
}

func decodeBody(data []byte, contentType string) any {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ""
	}
	if strings.Contains(contentType, "json") || trimmed[0] == '{' || trimmed[0] == '[' {
		var v any
		if err := json.Unmarshal(trimmed, &v); err == nil {
			return v
		}
	}
	return string(data)
}

func describeHTTPError(ctx context.Context, err error, timeout time.Duration) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, ErrLocalNetworkBlocked) {
		return ErrLocalNetworkBlocked
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("превышено время ожидания ответа (%s)", timeout)
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Errorf("не удалось найти хост %q", dnsErr.Name)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("ошибка запроса: %w", urlErr.Err)
	}
	return fmt.Errorf("ошибка запроса: %w", err)
}
