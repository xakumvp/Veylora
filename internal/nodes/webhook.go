package nodes

import (
	"context"
	"strings"
)

// TriggerWebhook is the source identifier for runs triggered by webhooks.
const TriggerWebhook = "webhook"

// WebhookTrigger starts a workflow when an external HTTP request hits the webhook server.
type WebhookTrigger struct{}

func (WebhookTrigger) Definition() Definition {
	return Definition{
		Type:        TypeWebhookTrigger,
		Name:        "Webhook Trigger",
		Category:    "Triggers",
		Description: "Запускает Workflow при поступлении внешнего HTTP-запроса (GET/POST/PUT/DELETE) на указанный эндпоинт.",
		Icon:        "webhook",
		KeyPrefix:   "trigger",
		Trigger:     TriggerWebhook,
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "endpoint",
				Label:       "URL эндпоинта",
				Kind:        ParamString,
				Required:    true,
				Default:     "my-webhook",
				Placeholder: "my-webhook",
				Description: "Имя эндпоинта. Обращение будет доступно по адресу /webhook/имя.",
			},
			{
				Key:      "method",
				Label:    "HTTP метод",
				Kind:     ParamSelect,
				Required: true,
				Default:  "POST",
				Options: []Option{
					{Value: "POST", Label: "POST"},
					{Value: "GET", Label: "GET"},
					{Value: "PUT", Label: "PUT"},
					{Value: "DELETE", Label: "DELETE"},
					{Value: "ANY", Label: "ANY (любой метод)"},
				},
			},
			{
				Key:      "authMode",
				Label:    "Авторизация",
				Kind:     ParamSelect,
				Required: true,
				Default:  "none",
				Options: []Option{
					{Value: "none", Label: "Без авторизации (публичный)"},
					{Value: "token", Label: "Bearer Token или ?token=... / ?secret=..."},
					{Value: "header", Label: "Пользовательский заголовок (X-Webhook-Secret)"},
				},
			},
			{
				Key:         "secret",
				Label:       "Секретный токен",
				Kind:        ParamString,
				Placeholder: "секретный-ключ-для-проверки",
				Description: "Значение токена авторизации (маскируется в логах).",
				OptionalWhen: &ParamCondition{
					Param: "authMode",
					In:    []string{"none"},
				},
			},
			{
				Key:         "headerName",
				Label:       "Имя заголовка авторизации",
				Kind:        ParamString,
				Default:     "X-Webhook-Secret",
				Placeholder: "X-Webhook-Secret",
				OptionalWhen: &ParamCondition{
					Param: "authMode",
					In:    []string{"none", "token"},
				},
			},
			{
				Key:         "responseCode",
				Label:       "HTTP статус ответа",
				Kind:        ParamNumber,
				Default:     200,
				Required:    true,
				Min:         ptr(200),
				Max:         ptr(299),
				Description: "Статус ответа вызывающей стороне (200 OK, 202 Accepted и т.д.).",
			},
			{
				Key:         "responseBody",
				Label:       "Тело ответа клиенту",
				Kind:        ParamText,
				Default:     `{"status":"ok","runId":"{{runId}}"}`,
				Placeholder: `{"status":"ok","runId":"{{runId}}"}`,
				Description: "Шаблон тела ответа (поддерживает подстановку {{runId}}).",
			},
		},
		OutputDoc: append([]Field{
			{Name: "method", Description: "HTTP метод входящего запроса (GET, POST и т.д.)"},
			{Name: "endpoint", Description: "Имя эндпоинта вебхука"},
			{Name: "path", Description: "Полный путь запроса"},
			{Name: "body", Description: "Распарсенное тело запроса (JSON-объект или строка)"},
			{Name: "rawBody", Description: "Сырое тело запроса"},
			{Name: "headers", Description: "Карта HTTP-заголовков ({{trigger.headers.content_type}})"},
			{Name: "query", Description: "Карта параметров строки запроса ({{trigger.query.paramName}})"},
			{Name: "ip", Description: "IP-адрес отправителя запроса"},
		}, triggerOutputDoc...),
	}
}

func (WebhookTrigger) Validate(p map[string]any) []FieldError {
	ep := strings.TrimSpace(Str(p, "endpoint", ""))
	if ep == "" {
		return []FieldError{{Field: "endpoint", Message: "эндпоинт вебхука не может быть пустым"}}
	}
	if strings.ContainsAny(ep, " ?#&") {
		return []FieldError{{Field: "endpoint", Message: "эндпоинт не должен содержать пробелы или спецсимволы"}}
	}
	return nil
}

func (WebhookTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	extra := map[string]any{}
	if in.Event == nil || in.Event.Source != TriggerWebhook {
		// Manual test run from editor
		ep := Str(in.Node.Params, "endpoint", "test")
		extra = map[string]any{
			"method":   Str(in.Node.Params, "method", "POST"),
			"endpoint": ep,
			"path":     "/webhook/" + ep,
			"body":     map[string]any{"test": true},
			"rawBody":  `{"test":true}`,
			"headers":  map[string]any{"content-type": "application/json"},
			"query":    map[string]any{},
			"ip":       "127.0.0.1",
		}
	} else if in.Event.Data != nil {
		extra = in.Event.Data
	}
	return Result{Output: triggerOutput(in, extra)}, nil
}
