package nodes

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------- JSON Parse ----------

type JSONParse struct{}

func (JSONParse) Definition() Definition {
	return Definition{
		Type:        TypeJSONParse,
		Name:        "JSON Parser",
		Category:    "Data Processing",
		Description: "Парсит строку JSON в структурированные данные. Поля становятся доступны через {{key.fieldName}}.",
		Icon:        "braces",
		KeyPrefix:   "json",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "text",
				Label:       "Строка JSON",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "{{http.body}}",
				Description: "Строка JSON для парсинга (например, тело ответа HTTP-запроса).",
			},
		},
		OutputDoc: []Field{
			{Name: "data", Description: "Распарсенный объект, массив или значение"},
			{Name: "type", Description: "Тип данных: object, array, string, number, boolean, null"},
			{Name: "length", Description: "Количество элементов для массива или ключей для объекта"},
		},
	}
}

func (JSONParse) Validate(p map[string]any) []FieldError {
	return nil
}

func (JSONParse) Execute(_ context.Context, in *Input) (Result, error) {
	raw := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "text", "")))
	if raw == "" {
		return Result{}, errors.New("пустая строка JSON")
	}

	var val any
	if err := json.Unmarshal([]byte(raw), &val); err != nil {
		return Result{}, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	out := map[string]any{"data": val}
	switch v := val.(type) {
	case map[string]any:
		out["type"] = "object"
		out["length"] = float64(len(v))
		for k, item := range v {
			if k != "data" && k != "type" && k != "length" {
				out[k] = item
			}
		}
	case []any:
		out["type"] = "array"
		out["length"] = float64(len(v))
	case string:
		out["type"] = "string"
	case float64:
		out["type"] = "number"
	case bool:
		out["type"] = "boolean"
	case nil:
		out["type"] = "null"
	default:
		out["type"] = "unknown"
	}

	return Result{Output: out}, nil
}

// ---------- JSON Build ----------

type JSONBuild struct{}

func (JSONBuild) Definition() Definition {
	return Definition{
		Type:        TypeJSONBuild,
		Name:        "JSON Builder",
		Category:    "Data Processing",
		Description: "Формирует JSON-строку из набора пар ключ-значение или по шаблону.",
		Icon:        "braces",
		KeyPrefix:   "json",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:      "mode",
				Label:    "Режим",
				Kind:     ParamSelect,
				Required: true,
				Default:  "fields",
				Options: []Option{
					{Value: "fields", Label: "Поля (Key-Value)"},
					{Value: "template", Label: "Шаблон JSON"},
				},
			},
			{
				Key:         "fields",
				Label:       "Поля",
				Kind:        ParamKeyValue,
				Templatable: true,
				Description: "Пары ключ-значение. Числа и логические значения приводятся автоматически.",
				OptionalWhen: &ParamCondition{
					Param: "mode",
					In:    []string{"template"},
				},
			},
			{
				Key:         "template",
				Label:       "Шаблон JSON",
				Kind:        ParamText,
				Templatable: true,
				Placeholder: "{\n  \"status\": \"ok\",\n  \"userId\": \"{{trigger.userId}}\"\n}",
				OptionalWhen: &ParamCondition{
					Param: "mode",
					In:    []string{"fields"},
				},
			},
			{
				Key:     "pretty",
				Label:   "Форматировать с отступами",
				Kind:    ParamBoolean,
				Default: true,
			},
		},
		OutputDoc: []Field{
			{Name: "json", Description: "Сформированная JSON-строка"},
		},
	}
}

func (JSONBuild) Validate(p map[string]any) []FieldError {
	mode := Str(p, "mode", "fields")
	if mode == "template" {
		tpl := strings.TrimSpace(Str(p, "template", ""))
		// If template doesn't contain variables, validate JSON immediately
		if tpl != "" && !strings.Contains(tpl, "{{") {
			var dummy any
			if err := json.Unmarshal([]byte(tpl), &dummy); err != nil {
				return []FieldError{{Field: "template", Message: "некорректный JSON в шаблоне"}}
			}
		}
	}
	return nil
}

func parseJSONValue(val any) any {
	s, ok := val.(string)
	if !ok {
		return val
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "true" {
		return true
	}
	if trimmed == "false" {
		return false
	}
	if trimmed == "null" {
		return nil
	}
	// Number
	if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
		if !strings.HasPrefix(trimmed, "0") || trimmed == "0" || strings.HasPrefix(trimmed, "0.") {
			return num
		}
	}
	// Nested JSON object or array
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var nested any
		if err := json.Unmarshal([]byte(trimmed), &nested); err == nil {
			return nested
		}
	}
	return s
}

func (JSONBuild) Execute(_ context.Context, in *Input) (Result, error) {
	mode := Str(in.Node.Params, "mode", "fields")
	pretty := Bool(in.Node.Params, "pretty", true)

	var resBytes []byte
	if mode == "template" {
		tpl := strings.TrimSpace(in.Vars.ResolveString(Str(in.Node.Params, "template", "{}")))
		if tpl == "" {
			tpl = "{}"
		}
		var parsed any
		if err := json.Unmarshal([]byte(tpl), &parsed); err != nil {
			return Result{}, fmt.Errorf("шаблон сформировал некорректный JSON: %w", err)
		}
		if pretty {
			b, err := json.MarshalIndent(parsed, "", "  ")
			if err != nil {
				return Result{}, err
			}
			resBytes = b
		} else {
			b, err := json.Marshal(parsed)
			if err != nil {
				return Result{}, err
			}
			resBytes = b
		}
	} else {
		obj := make(map[string]any)
		for _, kv := range KeyValues(in.Node.Params, "fields") {
			if !kv.Enabled || strings.TrimSpace(kv.Key) == "" {
				continue
			}
			rawVal := in.Vars.ResolveValue(kv.Value)
			obj[kv.Key] = parseJSONValue(rawVal)
		}
		var err error
		if pretty {
			resBytes, err = json.MarshalIndent(obj, "", "  ")
		} else {
			resBytes, err = json.Marshal(obj)
		}
		if err != nil {
			return Result{}, fmt.Errorf("ошибка сериализации JSON: %w", err)
		}
	}

	return Result{Output: map[string]any{"json": string(resBytes)}}, nil
}

// ---------- Regex ----------

type Regex struct{}

func (Regex) Definition() Definition {
	return Definition{
		Type:        TypeRegex,
		Name:        "Regex",
		Category:    "Data Processing",
		Description: "Поиск, извлечение групп и замена текста с помощью регулярных выражений.",
		Icon:        "regex",
		KeyPrefix:   "regex",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:      "action",
				Label:    "Действие",
				Kind:     ParamSelect,
				Required: true,
				Default:  "find",
				Options: []Option{
					{Value: "find", Label: "Найти первое совпадение"},
					{Value: "find_all", Label: "Найти все совпадения"},
					{Value: "replace", Label: "Заменить совпадения"},
					{Value: "test", Label: "Проверить наличие (Test)"},
				},
			},
			{
				Key:         "pattern",
				Label:       "Регулярное выражение",
				Kind:        ParamString,
				Required:    true,
				Placeholder: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
				Description: "Шаблон регулярного выражения (синтаксис Go / RE2).",
			},
			{
				Key:         "text",
				Label:       "Входной текст",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "Текст для поиска или замены",
			},
			{
				Key:         "replaceWith",
				Label:       "Заменить на",
				Kind:        ParamString,
				Templatable: true,
				Placeholder: "Новый текст",
				OptionalWhen: &ParamCondition{
					Param: "action",
					In:    []string{"find", "find_all", "test"},
				},
			},
		},
		OutputDoc: []Field{
			{Name: "matched", Description: "true, если найдено хотя бы одно совпадение"},
			{Name: "match", Description: "Первое найденное совпадение"},
			{Name: "matches", Description: "Список всех совпадений (для find_all)"},
			{Name: "groups", Description: "Карта именованных групп и список нумерованных групп"},
			{Name: "result", Description: "Результирующий текст после замены (для replace)"},
		},
	}
}

func (Regex) Validate(p map[string]any) []FieldError {
	pat := Str(p, "pattern", "")
	if strings.TrimSpace(pat) != "" {
		if _, err := regexp.Compile(pat); err != nil {
			return []FieldError{{Field: "pattern", Message: "некорректное регулярное выражение: " + err.Error()}}
		}
	}
	return nil
}

func (Regex) Execute(_ context.Context, in *Input) (Result, error) {
	pat := Str(in.Node.Params, "pattern", "")
	if strings.TrimSpace(pat) == "" {
		return Result{}, errors.New("не задан шаблон регулярного выражения")
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return Result{}, fmt.Errorf("некорректное регулярное выражение: %w", err)
	}

	text := in.Vars.ResolveString(Str(in.Node.Params, "text", ""))
	action := Str(in.Node.Params, "action", "find")

	out := map[string]any{
		"matched": false,
		"match":   "",
		"matches": []any{},
		"groups":  map[string]any{},
		"result":  text,
	}

	switch action {
	case "test":
		matched := re.MatchString(text)
		out["matched"] = matched

	case "find":
		submatches := re.FindStringSubmatch(text)
		if len(submatches) > 0 {
			out["matched"] = true
			out["match"] = submatches[0]
			groups := map[string]any{}
			names := re.SubexpNames()
			for i, name := range names {
				if i > 0 && i < len(submatches) {
					if name != "" {
						groups[name] = submatches[i]
					}
					groups[strconv.Itoa(i)] = submatches[i]
				}
			}
			out["groups"] = groups
		}

	case "find_all":
		all := re.FindAllString(text, -1)
		out["matched"] = len(all) > 0
		if len(all) > 0 {
			out["match"] = all[0]
			items := make([]any, len(all))
			for i, s := range all {
				items[i] = s
			}
			out["matches"] = items
		}

	case "replace":
		rep := in.Vars.ResolveString(Str(in.Node.Params, "replaceWith", ""))
		replaced := re.ReplaceAllString(text, rep)
		out["matched"] = re.MatchString(text)
		out["result"] = replaced

	default:
		return Result{}, fmt.Errorf("неизвестное действие: %s", action)
	}

	return Result{Output: out}, nil
}

// ---------- Base64 ----------

type Base64 struct{}

func (Base64) Definition() Definition {
	return Definition{
		Type:        TypeBase64,
		Name:        "Base64",
		Category:    "Data Processing",
		Description: "Кодирование и декодирование строк в формат Base64.",
		Icon:        "binary",
		KeyPrefix:   "b64",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:      "action",
				Label:    "Действие",
				Kind:     ParamSelect,
				Required: true,
				Default:  "encode",
				Options: []Option{
					{Value: "encode", Label: "Закодировать (Encode)"},
					{Value: "decode", Label: "Декодировать (Decode)"},
				},
			},
			{
				Key:         "text",
				Label:       "Текст",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "Строка для кодирования или декодирования",
			},
			{
				Key:     "urlSafe",
				Label:   "URL-safe алфавит (- и _ вместо + и /)",
				Kind:    ParamBoolean,
				Default: false,
			},
		},
		OutputDoc: []Field{
			{Name: "result", Description: "Результат кодирования или декодирования"},
		},
	}
}

func (Base64) Validate(p map[string]any) []FieldError {
	return nil
}

func (Base64) Execute(_ context.Context, in *Input) (Result, error) {
	action := Str(in.Node.Params, "action", "encode")
	urlSafe := Bool(in.Node.Params, "urlSafe", false)
	text := in.Vars.ResolveString(Str(in.Node.Params, "text", ""))

	var enc *base64.Encoding
	if urlSafe {
		enc = base64.URLEncoding
	} else {
		enc = base64.StdEncoding
	}

	if action == "decode" {
		decoded, err := enc.DecodeString(strings.TrimSpace(text))
		if err != nil {
			return Result{}, fmt.Errorf("ошибка декодирования Base64: %w", err)
		}
		return Result{Output: map[string]any{"result": string(decoded)}}, nil
	}

	encoded := enc.EncodeToString([]byte(text))
	return Result{Output: map[string]any{"result": encoded}}, nil
}

// ---------- Hash Generator ----------

type Hash struct{}

func (Hash) Definition() Definition {
	return Definition{
		Type:        TypeHash,
		Name:        "Hash Generator",
		Category:    "Data Processing",
		Description: "Вычисляет криптографический хеш от строки (MD5, SHA-1, SHA-256, SHA-512).",
		Icon:        "hash",
		KeyPrefix:   "hash",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:      "algorithm",
				Label:    "Алгоритм",
				Kind:     ParamSelect,
				Required: true,
				Default:  "sha256",
				Options: []Option{
					{Value: "sha256", Label: "SHA-256 (рекомендуется)"},
					{Value: "sha512", Label: "SHA-512"},
					{Value: "sha1", Label: "SHA-1"},
					{Value: "md5", Label: "MD5"},
				},
			},
			{
				Key:         "text",
				Label:       "Входной текст",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "Строка для хеширования",
			},
		},
		OutputDoc: []Field{
			{Name: "hash", Description: "Хеш в шестнадцатеричном виде (hex)"},
		},
	}
}

func (Hash) Validate(p map[string]any) []FieldError {
	return nil
}

func (Hash) Execute(_ context.Context, in *Input) (Result, error) {
	algo := Str(in.Node.Params, "algorithm", "sha256")
	text := in.Vars.ResolveString(Str(in.Node.Params, "text", ""))

	var hashBytes []byte
	switch strings.ToLower(algo) {
	case "sha256":
		h := sha256.Sum256([]byte(text))
		hashBytes = h[:]
	case "sha512":
		h := sha512.Sum512([]byte(text))
		hashBytes = h[:]
	case "sha1":
		h := sha1.Sum([]byte(text))
		hashBytes = h[:]
	case "md5":
		h := md5.Sum([]byte(text))
		hashBytes = h[:]
	default:
		return Result{}, fmt.Errorf("неподдерживаемый алгоритм хеширования: %s", algo)
	}

	return Result{Output: map[string]any{"hash": hex.EncodeToString(hashBytes)}}, nil
}

// ---------- Date / Time ----------

type DateTime struct{}

func (DateTime) Definition() Definition {
	return Definition{
		Type:        TypeDateTime,
		Name:        "Date / Time",
		Category:    "Data Processing",
		Description: "Получение текущего времени, парсинг, арифметические операции и форматирование дат.",
		Icon:        "calendar",
		KeyPrefix:   "date",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "input",
				Label:       "Исходная дата / время",
				Kind:        ParamString,
				Templatable: true,
				Placeholder: "Пусто для текущего времени или 2026-11-13T08:00:00Z / 1731484800",
				Description: "Оставьте пустым для 'сейчас', либо передайте ISO-строку или unix timestamp.",
			},
			{
				Key:         "format",
				Label:       "Формат вывода",
				Kind:        ParamString,
				Default:     "2006-01-02 15:04:05",
				Placeholder: "2006-01-02 15:04:05, iso, unix",
				Description: "Go-формат даты (2006-01-02 15:04:05) либо 'iso' или 'unix'.",
			},
			{Key: "addDays", Label: "Добавить дней", Kind: ParamNumber, Default: 0},
			{Key: "addHours", Label: "Добавить часов", Kind: ParamNumber, Default: 0},
			{Key: "addMinutes", Label: "Добавить минут", Kind: ParamNumber, Default: 0},
			{Key: "addSeconds", Label: "Добавить секунд", Kind: ParamNumber, Default: 0},
		},
		OutputDoc: []Field{
			{Name: "formatted", Description: "Отформатированная дата"},
			{Name: "iso", Description: "Дата в стандарте ISO 8601 / RFC3339"},
			{Name: "unix", Description: "Unix timestamp в секундах"},
			{Name: "unixMs", Description: "Unix timestamp в миллисекундах"},
			{Name: "year", Description: "Год"},
			{Name: "month", Description: "Месяц (1-12)"},
			{Name: "day", Description: "День месяца"},
			{Name: "hour", Description: "Час (0-23)"},
			{Name: "minute", Description: "Минута"},
			{Name: "second", Description: "Секунда"},
		},
	}
}

func (DateTime) Validate(p map[string]any) []FieldError {
	return nil
}

func parseDateInput(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now(), nil
	}
	// Try parsing unix timestamp (seconds or millis)
	if num, err := strconv.ParseInt(s, 10, 64); err == nil {
		if num > 1e11 {
			return time.UnixMilli(num), nil
		}
		return time.Unix(num, 0), nil
	}
	// Try standard layouts
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"02.01.2006 15:04:05",
		"02.01.2006",
		time.RFC1123Z,
		time.RFC1123,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("не удалось распознать формат даты: %q", s)
}

func (DateTime) Execute(_ context.Context, in *Input) (Result, error) {
	rawInput := in.Vars.ResolveString(Str(in.Node.Params, "input", ""))
	t, err := parseDateInput(rawInput)
	if err != nil {
		return Result{}, err
	}

	// Apply offsets
	days, _ := Num(in.Node.Params, "addDays")
	hours, _ := Num(in.Node.Params, "addHours")
	minutes, _ := Num(in.Node.Params, "addMinutes")
	seconds, _ := Num(in.Node.Params, "addSeconds")

	if days != 0 {
		t = t.AddDate(0, 0, int(days))
	}
	if hours != 0 || minutes != 0 || seconds != 0 {
		d := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
		t = t.Add(d)
	}

	fmtStr := strings.TrimSpace(Str(in.Node.Params, "format", "2006-01-02 15:04:05"))
	var formatted string
	switch strings.ToLower(fmtStr) {
	case "iso", "rfc3339":
		formatted = t.Format(time.RFC3339)
	case "unix":
		formatted = strconv.FormatInt(t.Unix(), 10)
	case "unixms":
		formatted = strconv.FormatInt(t.UnixMilli(), 10)
	default:
		formatted = t.Format(fmtStr)
	}

	out := map[string]any{
		"formatted": formatted,
		"iso":       t.Format(time.RFC3339),
		"unix":      float64(t.Unix()),
		"unixMs":    float64(t.UnixMilli()),
		"year":      float64(t.Year()),
		"month":     float64(t.Month()),
		"day":       float64(t.Day()),
		"hour":      float64(t.Hour()),
		"minute":    float64(t.Minute()),
		"second":    float64(t.Second()),
	}
	return Result{Output: out}, nil
}
