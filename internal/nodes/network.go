package nodes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Network node types.
const (
	TypeTCPCheck  = "network.tcp_check"
	TypeDNSLookup = "network.dns"
	TypeURLParse  = "network.url_parse"
)

// ----------------------------------------------------------------------------
// 1. TCP Check Node
// ----------------------------------------------------------------------------

// TCPCheckNode verifies if a remote or local TCP port is reachable.
type TCPCheckNode struct{}

func (TCPCheckNode) Definition() Definition {
	return Definition{
		Type:        TypeTCPCheck,
		Name:        "TCP Check",
		Category:    "Network",
		Description: "Проверяет доступность сервиса по протоколу TCP (health check хоста и порта) с замером времени отклика.",
		Icon:        "network",
		KeyPrefix:   "tcp",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "host",
				Label:       "Хост / IP-адрес",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "example.com или 1.1.1.1",
				Description: "Целевой хост или IP-адрес для проверки соединения.",
			},
			{
				Key:         "port",
				Label:       "Порт TCP",
				Kind:        ParamNumber,
				Required:    true,
				Default:     80,
				Min:         ptr(1),
				Max:         ptr(65535),
				Description: "Целевой сетевой порт (1..65535).",
			},
			{
				Key:         "timeout",
				Label:       "Таймаут (мс)",
				Kind:        ParamNumber,
				Default:     3000,
				Min:         ptr(100),
				Max:         ptr(30000),
				Description: "Максимальное время ожидания ответа в миллисекундах.",
			},
		},
		OutputDoc: []Field{
			{Name: "alive", Description: "true, если порт открыт и ответил на TCP-соединение"},
			{Name: "latencyMs", Description: "Время установления соединения в миллисекундах"},
			{Name: "address", Description: "Проверенный адрес (хост:порт)"},
			{Name: "error", Description: "Текст ошибки, если соединение не удалось установить"},
		},
	}
}

func (TCPCheckNode) Validate(p map[string]any) []FieldError {
	var errs []FieldError
	if strings.TrimSpace(Str(p, "host", "")) == "" {
		errs = append(errs, FieldError{Field: "host", Message: "укажите хост для проверки"})
	}
	port := int(NumDefault(p, "port", 80))
	if port < 1 || port > 65535 {
		errs = append(errs, FieldError{Field: "port", Message: "порт должен быть в диапазоне от 1 до 65535"})
	}
	return errs
}

func (TCPCheckNode) Execute(ctx context.Context, in *Input) (Result, error) {
	p := in.Node.Params
	host := strings.TrimSpace(in.Vars.ResolveString(Str(p, "host", "")))
	if host == "" {
		return Result{}, errors.New("хост не может быть пустым")
	}

	port := int(NumDefault(p, "port", 80))
	timeoutMs := time.Duration(NumDefault(p, "timeout", 3000)) * time.Millisecond
	if timeoutMs < 100*time.Millisecond {
		timeoutMs = 100 * time.Millisecond
	}
	if timeoutMs > 30*time.Second {
		timeoutMs = 30 * time.Second
	}

	// Security check for local network
	if !in.Env.AllowLocalNetwork {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err == nil {
			for _, ip := range ips {
				if isLocalIP(ip) {
					return Result{}, ErrLocalNetworkBlocked
				}
			}
		} else {
			// If it's a literal IP string
			if ip := net.ParseIP(host); ip != nil && isLocalIP(ip) {
				return Result{}, ErrLocalNetworkBlocked
			}
		}
	}

	targetAddr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	in.Log(LevelDebug, fmt.Sprintf("TCP Check: подключение к %s (timeout: %s)", targetAddr, timeoutMs), nil)

	start := time.Now()
	dialer := net.Dialer{Timeout: timeoutMs}
	conn, err := dialer.DialContext(ctx, "tcp", targetAddr)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		in.Log(LevelInfo, fmt.Sprintf("TCP Check: %s недоступен (%v, %d мс)", targetAddr, err, latency), nil)
		return Result{
			Output: map[string]any{
				"alive":     false,
				"latencyMs": latency,
				"address":   targetAddr,
				"error":     err.Error(),
			},
		}, nil
	}
	defer conn.Close()

	in.Log(LevelInfo, fmt.Sprintf("TCP Check: %s доступен (отклик %d мс)", targetAddr, latency), nil)
	return Result{
		Output: map[string]any{
			"alive":     true,
			"latencyMs": latency,
			"address":   targetAddr,
			"error":     "",
		},
	}, nil
}

// ----------------------------------------------------------------------------
// 2. DNS Lookup Node
// ----------------------------------------------------------------------------

// DNSLookupNode queries DNS records (A, AAAA, CNAME, MX, TXT) for a domain.
type DNSLookupNode struct{}

func (DNSLookupNode) Definition() Definition {
	return Definition{
		Type:        TypeDNSLookup,
		Name:        "DNS Lookup",
		Category:    "Network",
		Description: "Разрешает ресурсные записи DNS (A, AAAA, CNAME, MX, TXT) для указанного домена.",
		Icon:        "globe",
		KeyPrefix:   "dns",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "domain",
				Label:       "Доменное имя",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "google.com",
				Description: "Имя домена для DNS-запроса.",
			},
			{
				Key:      "recordType",
				Label:    "Тип записи",
				Kind:     ParamSelect,
				Required: true,
				Default:  "A",
				Options: []Option{
					{Value: "A", Label: "A (IPv4 адрес)"},
					{Value: "AAAA", Label: "AAAA (IPv6 адрес)"},
					{Value: "CNAME", Label: "CNAME (Каноническое имя)"},
					{Value: "MX", Label: "MX (Почтовые серверы)"},
					{Value: "TXT", Label: "TXT (Текстовые записи / SPF / DKIM)"},
				},
			},
		},
		OutputDoc: []Field{
			{Name: "records", Description: "Список найденных значений DNS-записей"},
			{Name: "count", Description: "Количество найденных записей"},
			{Name: "first", Description: "Первая запись (или пустая строка)"},
			{Name: "recordType", Description: "Запрошенный тип записи"},
			{Name: "domain", Description: "Запрошенный домен"},
		},
	}
}

func (DNSLookupNode) Validate(p map[string]any) []FieldError {
	if strings.TrimSpace(Str(p, "domain", "")) == "" {
		return []FieldError{{Field: "domain", Message: "укажите доменное имя"}}
	}
	return nil
}

func (DNSLookupNode) Execute(ctx context.Context, in *Input) (Result, error) {
	domain := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "domain", "")))
	recType := strings.ToUpper(Str(in.Node.Params, "recordType", "A"))

	if domain == "" {
		return Result{}, errors.New("доменное имя не может быть пустым")
	}

	in.Log(LevelDebug, fmt.Sprintf("DNS Lookup: %s [%s]", domain, recType), nil)

	var records []string
	switch recType {
	case "A":
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", domain)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка DNS A для %q: %w", domain, err)
		}
		for _, ip := range ips {
			records = append(records, ip.String())
		}

	case "AAAA":
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip6", domain)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка DNS AAAA для %q: %w", domain, err)
		}
		for _, ip := range ips {
			records = append(records, ip.String())
		}

	case "CNAME":
		cname, err := net.DefaultResolver.LookupCNAME(ctx, domain)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка DNS CNAME для %q: %w", domain, err)
		}
		if cname != "" {
			records = append(records, strings.TrimSuffix(cname, "."))
		}

	case "MX":
		mxList, err := net.DefaultResolver.LookupMX(ctx, domain)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка DNS MX для %q: %w", domain, err)
		}
		for _, mx := range mxList {
			records = append(records, fmt.Sprintf("%d %s", mx.Pref, strings.TrimSuffix(mx.Host, ".")))
		}

	case "TXT":
		txts, err := net.DefaultResolver.LookupTXT(ctx, domain)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка DNS TXT для %q: %w", domain, err)
		}
		records = txts

	default:
		return Result{}, fmt.Errorf("неподдерживаемый тип DNS-записи %q", recType)
	}

	first := ""
	if len(records) > 0 {
		first = records[0]
	} else {
		records = []string{}
	}

	in.Log(LevelInfo, fmt.Sprintf("DNS Lookup: найдено %d записей для %s [%s]", len(records), domain, recType), nil)
	return Result{
		Output: map[string]any{
			"records":    records,
			"count":      len(records),
			"first":      first,
			"recordType": recType,
			"domain":     domain,
		},
	}, nil
}

// ----------------------------------------------------------------------------
// 3. URL Parser Node
// ----------------------------------------------------------------------------

// URLParseNode parses a full URL into structured protocol, host, port, path, and query parts.
type URLParseNode struct{}

func (URLParseNode) Definition() Definition {
	return Definition{
		Type:        TypeURLParse,
		Name:        "URL Parser",
		Category:    "Network",
		Description: "Разбирает веб-адрес (URL) на компоненты: протокол, домен, порт, путь, параметры строки запроса и фрагмент.",
		Icon:        "globe",
		KeyPrefix:   "url",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "url",
				Label:       "URL адрес",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "https://user:pass@example.com:8080/path/page?id=42#section",
				Description: "Полный URL для анализа.",
			},
		},
		OutputDoc: []Field{
			{Name: "protocol", Description: "Схема URL (http, https, ftp и т.д.)"},
			{Name: "host", Description: "Хост с портом (например: example.com:8080)"},
			{Name: "hostname", Description: "Только домен или IP (например: example.com)"},
			{Name: "port", Description: "Номер порта строкой (или пустая строка при стандартном порте)"},
			{Name: "pathname", Description: "Путь URL (например: /path/page)"},
			{Name: "search", Description: "Строка параметров с вопросительным знаком (?id=42)"},
			{Name: "hash", Description: "Хеш / фрагмент якоря (#section)"},
			{Name: "query", Description: "Словарь разобранных query-параметров ({{url.query.id}})"},
			{Name: "username", Description: "Имя пользователя при наличии Basic Auth"},
			{Name: "valid", Description: "Признак валидности URL"},
		},
	}
}

func (URLParseNode) Validate(p map[string]any) []FieldError {
	if strings.TrimSpace(Str(p, "url", "")) == "" {
		return []FieldError{{Field: "url", Message: "укажите URL для разбора"}}
	}
	return nil
}

func (URLParseNode) Execute(_ context.Context, in *Input) (Result, error) {
	rawURL := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "url", "")))
	if rawURL == "" {
		return Result{}, errors.New("URL не может быть пустым")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Result{
			Output: map[string]any{
				"valid": false,
				"error": err.Error(),
			},
		}, fmt.Errorf("некорректный URL %q: %w", rawURL, err)
	}

	queryParams := map[string]any{}
	for k, v := range parsed.Query() {
		if len(v) == 1 {
			queryParams[k] = v[0]
		} else {
			queryParams[k] = v
		}
	}

	username := ""
	if parsed.User != nil {
		username = parsed.User.Username()
	}

	search := ""
	if parsed.RawQuery != "" {
		search = "?" + parsed.RawQuery
	}

	hash := ""
	if parsed.Fragment != "" {
		hash = "#" + parsed.Fragment
	}

	return Result{
		Output: map[string]any{
			"protocol": parsed.Scheme,
			"host":     parsed.Host,
			"hostname": parsed.Hostname(),
			"port":     parsed.Port(),
			"pathname": parsed.Path,
			"search":   search,
			"hash":     hash,
			"query":    queryParams,
			"username": username,
			"valid":    true,
		},
	}, nil
}
