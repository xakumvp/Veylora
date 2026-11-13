package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ---------- Telegram ----------

type Telegram struct{}

func (Telegram) Definition() Definition {
	return Definition{
		Type:        TypeTelegram,
		Name:        "Telegram",
		Category:    "Notifications",
		Description: "Отправляет текстовое сообщение в Telegram через Bot API.",
		Icon:        "send",
		KeyPrefix:   "telegram",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "botToken",
				Label:       "Токен бота",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
				Description: "Токен Telegram-бота из @BotFather (маскируется в логах и экспорте).",
			},
			{
				Key:         "chatId",
				Label:       "ID чата",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "-1001234567890 или 123456789 или @channel",
				Description: "Числовой ID чата/пользователя или @username публичного канала.",
			},
			{
				Key:         "message",
				Label:       "Сообщение",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "Уведомление от Veylora: {{workflow.name}} успешно выполнен!",
			},
			{
				Key:      "parseMode",
				Label:    "Форматирование текста",
				Kind:     ParamSelect,
				Default:  "HTML",
				Required: true,
				Options: []Option{
					{Value: "HTML", Label: "HTML (<b>жирный</b>, <i>курсив</i>, <code>код</code>)"},
					{Value: "MarkdownV2", Label: "MarkdownV2 (*жирный*, _курсив_)"},
					{Value: "Markdown", Label: "Markdown (устаревший)"},
					{Value: "None", Label: "Без форматирования (обычный текст)"},
				},
			},
			{
				Key:     "disableNotification",
				Label:   "Отправить без звука (Silent)",
				Kind:    ParamBoolean,
				Default: false,
			},
		},
		OutputDoc: []Field{
			{Name: "success", Description: "true, если сообщение успешно отправлено"},
			{Name: "messageId", Description: "ID отправленного сообщения в Telegram"},
			{Name: "chatId", Description: "ID чата получателя"},
		},
	}
}

func (Telegram) Validate(p map[string]any) []FieldError {
	return nil
}

type telegramResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
	Result      struct {
		MessageID int `json:"message_id"`
	} `json:"result,omitempty"`
}

func (Telegram) Execute(ctx context.Context, in *Input) (Result, error) {
	token := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "botToken", "")))
	if token == "" {
		return Result{}, errors.New("не указан токен Telegram-бота")
	}
	if in.Redactor != nil {
		in.Redactor.Add(token)
	}

	chatId := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "chatId", "")))
	if chatId == "" {
		return Result{}, errors.New("не указан ID чата Telegram")
	}

	msg := in.Vars.ResolveString(Str(in.Node.Params, "message", ""))
	if strings.TrimSpace(msg) == "" {
		return Result{}, errors.New("сообщение не может быть пустым")
	}

	parseMode := Str(in.Node.Params, "parseMode", "HTML")
	disableNotification := Bool(in.Node.Params, "disableNotification", false)

	reqPayload := map[string]any{
		"chat_id":              chatId,
		"text":                 msg,
		"disable_notification": disableNotification,
	}
	if parseMode != "None" && parseMode != "" {
		reqPayload["parse_mode"] = parseMode
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return Result{}, fmt.Errorf("ошибка формирования запроса: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return Result{}, fmt.Errorf("не удалось создать запрос к Telegram: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Veylora/0.2")

	client := newHTTPClient(in.Env, 20*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ошибка отправки в Telegram: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("ошибка чтения ответа от Telegram: %w", err)
	}

	var tgResp telegramResponse
	if err := json.Unmarshal(respData, &tgResp); err != nil {
		return Result{}, fmt.Errorf("некорректный ответ Telegram API (статус %d)", resp.StatusCode)
	}

	if !tgResp.OK || resp.StatusCode != http.StatusOK {
		desc := tgResp.Description
		if desc == "" {
			desc = http.StatusText(resp.StatusCode)
		}
		return Result{}, fmt.Errorf("Telegram API ошибка (%d): %s", tgResp.ErrorCode, desc)
	}

	out := map[string]any{
		"success":   true,
		"messageId": float64(tgResp.Result.MessageID),
		"chatId":    chatId,
	}
	return Result{Output: out}, nil
}

// ---------- Discord ----------

type Discord struct{}

func (Discord) Definition() Definition {
	return Definition{
		Type:        TypeDiscord,
		Name:        "Discord",
		Category:    "Notifications",
		Description: "Отправляет сообщение в канал Discord через Webhook.",
		Icon:        "message-square",
		KeyPrefix:   "discord",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "webhookUrl",
				Label:       "Discord Webhook URL",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "https://discord.com/api/webhooks/...",
				Description: "URL вебхука канала Discord (маскируется в логах и экспорте).",
			},
			{
				Key:         "content",
				Label:       "Сообщение",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "Уведомление от Veylora: {{workflow.name}}",
			},
			{
				Key:         "username",
				Label:       "Имя бота (Username)",
				Kind:        ParamString,
				Templatable: true,
				Placeholder: "Veylora Bot (по умолчанию имя из вебхука)",
			},
			{
				Key:         "avatarUrl",
				Label:       "URL аватарки",
				Kind:        ParamString,
				Templatable: true,
				Placeholder: "https://example.com/avatar.png",
			},
		},
		OutputDoc: []Field{
			{Name: "success", Description: "true, если сообщение успешно отправлено"},
			{Name: "status", Description: "HTTP статус ответа Discord (обычно 204)"},
		},
	}
}

func (Discord) Validate(p map[string]any) []FieldError {
	u := Str(p, "webhookUrl", "")
	if strings.TrimSpace(u) != "" && !strings.Contains(u, "{{") {
		if !strings.HasPrefix(u, "https://discord.com/api/webhooks/") &&
			!strings.HasPrefix(u, "https://discordapp.com/api/webhooks/") {
			return []FieldError{{Field: "webhookUrl", Message: "URL должен начинаться с https://discord.com/api/webhooks/"}}
		}
	}
	return nil
}

func (Discord) Execute(ctx context.Context, in *Input) (Result, error) {
	webhookUrl := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "webhookUrl", "")))
	if webhookUrl == "" {
		return Result{}, errors.New("не указан URL вебхука Discord")
	}
	if in.Redactor != nil {
		in.Redactor.Add(webhookUrl)
	}

	content := in.Vars.ResolveString(Str(in.Node.Params, "content", ""))
	if strings.TrimSpace(content) == "" {
		return Result{}, errors.New("сообщение для Discord не может быть пустым")
	}

	payload := map[string]any{
		"content": content,
	}
	if username := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "username", ""))); username != "" {
		payload["username"] = username
	}
	if avatarUrl := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "avatarUrl", ""))); avatarUrl != "" {
		payload["avatar_url"] = avatarUrl
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("ошибка формирования тела запроса: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookUrl, bytes.NewReader(bodyBytes))
	if err != nil {
		return Result{}, fmt.Errorf("не удалось создать запрос к Discord: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Veylora/0.2")

	client := newHTTPClient(in.Env, 20*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ошибка отправки в Discord: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyData, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Result{}, fmt.Errorf("Discord вернул ошибку %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyData)))
	}

	return Result{Output: map[string]any{
		"success": true,
		"status":  float64(resp.StatusCode),
	}}, nil
}
