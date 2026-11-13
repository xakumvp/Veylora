package nodes

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// TypeEmail identifies the SMTP Email notification node.
const TypeEmail = "notification.email"

// EmailNode sends notifications via SMTP servers.
type EmailNode struct{}

func (EmailNode) Definition() Definition {
	return Definition{
		Type:        TypeEmail,
		Name:        "Email (SMTP)",
		Category:    "Notifications",
		Description: "Отправляет электронное письмо через SMTP-сервер с поддержкой TLS/STARTTLS и HTML-разметки.",
		Icon:        "mail",
		KeyPrefix:   "email",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "smtpHost",
				Label:       "SMTP Сервер",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "smtp.example.com",
				Description: "Адрес почтового сервера.",
			},
			{
				Key:         "smtpPort",
				Label:       "Порт SMTP",
				Kind:        ParamNumber,
				Required:    true,
				Default:     587,
				Min:         ptr(1),
				Max:         ptr(65535),
				Description: "587 для STARTTLS, 465 для SSL/TLS, 25 для стандартного SMTP.",
			},
			{
				Key:         "username",
				Label:       "Имя пользователя / Логин",
				Kind:        ParamString,
				Templatable: true,
				Placeholder: "user@example.com",
			},
			{
				Key:         "password",
				Label:       "Пароль",
				Kind:        ParamString,
				Templatable: true,
				Placeholder: "••••••••",
				Description: "Пароль или пароль приложения (маскируется в логах).",
			},
			{
				Key:         "from",
				Label:       "Отправитель (From)",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "no-reply@example.com",
			},
			{
				Key:         "to",
				Label:       "Получатели (To)",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "user@example.com, admin@example.com",
				Description: "Список адресов получателей через запятую.",
			},
			{
				Key:         "subject",
				Label:       "Тема письма",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "Уведомление от Veylora: {{trigger.runName}}",
			},
			{
				Key:         "body",
				Label:       "Текст письма",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "Результат выполнения: {{json.status}}",
			},
			{
				Key:         "isHtml",
				Label:       "Формат HTML",
				Kind:        ParamBoolean,
				Default:     false,
				Description: "Отправлять письмо как text/html вместо обычного text/plain.",
			},
			{
				Key:      "tlsMode",
				Label:    "Режим шифрования",
				Kind:     ParamSelect,
				Required: true,
				Default:  "starttls",
				Options: []Option{
					{Value: "starttls", Label: "STARTTLS (порт 587 / 25)"},
					{Value: "tls", Label: "SSL/TLS (порт 465)"},
					{Value: "none", Label: "Без шифрования (Plain)"},
				},
			},
		},
		OutputDoc: []Field{
			{Name: "sent", Description: "true, если письмо успешно принято SMTP-сервером"},
			{Name: "recipients", Description: "Список адресов, которым отправлено письмо"},
			{Name: "recipientCount", Description: "Количество получателей"},
			{Name: "subject", Description: "Отправленная тема письма"},
		},
	}
}

func (EmailNode) Validate(p map[string]any) []FieldError {
	var errs []FieldError
	if strings.TrimSpace(Str(p, "smtpHost", "")) == "" {
		errs = append(errs, FieldError{Field: "smtpHost", Message: "укажите адрес SMTP-сервера"})
	}
	if strings.TrimSpace(Str(p, "from", "")) == "" {
		errs = append(errs, FieldError{Field: "from", Message: "укажите адрес отправителя"})
	}
	if strings.TrimSpace(Str(p, "to", "")) == "" {
		errs = append(errs, FieldError{Field: "to", Message: "укажите хотя бы одного получателя"})
	}
	if strings.TrimSpace(Str(p, "subject", "")) == "" {
		errs = append(errs, FieldError{Field: "subject", Message: "укажите тему письма"})
	}
	return errs
}

func (EmailNode) Execute(ctx context.Context, in *Input) (Result, error) {
	p := in.Node.Params
	host := strings.TrimSpace(in.Vars.ResolveString(Str(p, "smtpHost", "")))
	port := int(NumDefault(p, "smtpPort", 587))
	username := strings.TrimSpace(in.Vars.ResolveString(Str(p, "username", "")))
	password := in.Vars.ResolveString(Str(p, "password", ""))
	from := strings.TrimSpace(in.Vars.ResolveString(Str(p, "from", "")))
	rawTo := in.Vars.ResolveString(Str(p, "to", ""))
	subject := in.Vars.ResolveString(Str(p, "subject", ""))
	body := in.Vars.ResolveString(Str(p, "body", ""))
	isHTML := Bool(p, "isHtml", false)
	tlsMode := strings.ToLower(Str(p, "tlsMode", "starttls"))

	if password != "" {
		in.Redactor.Add(password)
	}

	if host == "" {
		return Result{}, errors.New("адрес SMTP-сервера не задан")
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
		} else if ip := net.ParseIP(host); ip != nil && isLocalIP(ip) {
			return Result{}, ErrLocalNetworkBlocked
		}
	}

	var recipients []string
	for _, addr := range strings.Split(rawTo, ",") {
		clean := strings.TrimSpace(addr)
		if clean != "" {
			recipients = append(recipients, clean)
		}
	}
	if len(recipients) == 0 {
		return Result{}, errors.New("список получателей пуст")
	}

	targetAddr := fmt.Sprintf("%s:%d", host, port)
	in.Log(LevelDebug, fmt.Sprintf("Отправка Email: %s -> %v через %s", from, recipients, targetAddr), nil)

	// Build MIME message
	contentType := "text/plain; charset=UTF-8"
	if isHTML {
		contentType = "text/html; charset=UTF-8"
	}

	header := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\nDate: %s\r\n\r\n",
		from, strings.Join(recipients, ", "), subject, contentType, time.Now().Format(time.RFC1123Z))
	msg := []byte(header + body)

	// Configure TLS
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	var client *smtp.Client
	timeout := 15 * time.Second

	switch tlsMode {
	case "tls":
		dialer := &net.Dialer{Timeout: timeout}
		tlsConn, err := tls.DialWithDialer(dialer, "tcp", targetAddr, tlsConfig)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка TLS-подключения к %s: %w", targetAddr, err)
		}
		c, err := smtp.NewClient(tlsConn, host)
		if err != nil {
			tlsConn.Close()
			return Result{}, fmt.Errorf("ошибка создания SMTP клиента: %w", err)
		}
		client = c

	case "none":
		conn, err := net.DialTimeout("tcp", targetAddr, timeout)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка подключения к %s: %w", targetAddr, err)
		}
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return Result{}, fmt.Errorf("ошибка создания SMTP клиента: %w", err)
		}
		client = c

	default: // starttls
		conn, err := net.DialTimeout("tcp", targetAddr, timeout)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка подключения к %s: %w", targetAddr, err)
		}
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return Result{}, fmt.Errorf("ошибка создания SMTP клиента: %w", err)
		}
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsConfig); err != nil {
				c.Close()
				return Result{}, fmt.Errorf("ошибка STARTTLS: %w", err)
			}
		}
		client = c
	}
	defer client.Close()

	// Authenticate if credentials provided
	if username != "" && password != "" {
		auth := smtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return Result{}, fmt.Errorf("ошибка аутентификации SMTP: %w", err)
		}
	}

	// Send mail
	if err := client.Mail(from); err != nil {
		return Result{}, fmt.Errorf("ошибка команды MAIL FROM: %w", err)
	}
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return Result{}, fmt.Errorf("ошибка команды RCPT TO для %q: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return Result{}, fmt.Errorf("ошибка открытия потока DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return Result{}, fmt.Errorf("ошибка записи тела письма: %w", err)
	}
	if err := w.Close(); err != nil {
		return Result{}, fmt.Errorf("ошибка завершения отправки письма: %w", err)
	}

	_ = client.Quit()

	in.Log(LevelInfo, fmt.Sprintf("Email успешно отправлен: %s (получателей: %d)", subject, len(recipients)), nil)
	return Result{
		Output: map[string]any{
			"sent":           true,
			"recipients":     recipients,
			"recipientCount": len(recipients),
			"subject":        subject,
		},
	}, nil
}
