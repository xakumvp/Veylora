package nodes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Trigger kinds.
const (
	TriggerManual   = "manual"
	TriggerCron     = "cron"
	TriggerInterval = "interval"
	TriggerSchedule = "schedule"
	TriggerFile     = "file"
	TriggerStartup  = "startup"
)

// MinInterval protects against accidental tight loops.
const MinInterval = 10 * time.Second

var triggerOutputDoc = []Field{
	{Name: "firedAt", Description: "Время срабатывания (RFC 3339)"},
	{Name: "runId", Description: "ID выполнения"},
	{Name: "source", Description: "Источник: manual, schedule, file, startup"},
}

// triggerOutput merges the common trigger fields with event data.
func triggerOutput(in *Input, extra map[string]any) map[string]any {
	out := map[string]any{
		"firedAt": time.Now().Format(time.RFC3339),
		"runId":   in.RunID,
		"source":  TriggerManual,
	}
	if in.Event != nil {
		if in.Event.Source != "" {
			out["source"] = in.Event.Source
		}
		for k, v := range in.Event.Data {
			out[k] = v
		}
	}
	for k, v := range extra {
		if _, set := out[k]; !set {
			out[k] = v
		}
	}
	return out
}

var timezoneParam = ParamSpec{Key: "timezone", Label: "Часовой пояс", Kind: ParamString, Placeholder: "Europe/Moscow",
	Description: "IANA-имя, например Europe/Moscow или UTC. Пусто — локальное время компьютера."}

// LoadLocation resolves a timezone parameter ("" = local time).
func LoadLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "local") {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("неизвестный часовой пояс %q", name)
	}
	return loc, nil
}

// ---------- Manual ----------

// ManualTrigger starts a workflow when the user presses Run.
type ManualTrigger struct{}

func (ManualTrigger) Definition() Definition {
	return Definition{
		Type:        TypeManualTrigger,
		Name:        "Manual Trigger",
		Category:    "Triggers",
		Description: "Запускает Workflow вручную по кнопке Run.",
		Icon:        "play",
		KeyPrefix:   "trigger",
		Trigger:     TriggerManual,
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "runName", Label: "Название запуска", Kind: ParamString, Placeholder: "Ручной запуск", Default: "Ручной запуск"},
		},
		OutputDoc: append([]Field{{Name: "runName", Description: "Название запуска"}, {Name: "startedAt", Description: "Время запуска"}, {Name: "status", Description: "Статус запуска"}}, triggerOutputDoc...),
	}
}

func (ManualTrigger) Validate(map[string]any) []FieldError { return nil }

func (ManualTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	now := time.Now().Format(time.RFC3339)
	return Result{Output: triggerOutput(in, map[string]any{
		"runName":   Str(in.Node.Params, "runName", "Ручной запуск"),
		"startedAt": now,
		"status":    "started",
	})}, nil
}

// ---------- Cron ----------

// CronParser accepts standard 5-field expressions and descriptors like @daily.
var CronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// CronTrigger fires on a cron expression.
type CronTrigger struct{}

func (CronTrigger) Definition() Definition {
	return Definition{
		Type:        TypeCronTrigger,
		Name:        "Cron Trigger",
		Category:    "Triggers",
		Description: "Запускает Workflow по cron-расписанию, когда Workflow активен.",
		Icon:        "calendar-clock",
		KeyPrefix:   "cron",
		Trigger:     TriggerCron,
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "expression", Label: "Cron-выражение", Kind: ParamString, Required: true, Default: "0 9 * * 1-5", Placeholder: "0 9 * * 1-5",
				Description: "минуты часы день месяц день_недели. Пример: 0 9 * * 1-5 — в 9:00 по будням. Также @hourly, @daily."},
			timezoneParam,
		},
		OutputDoc: append([]Field{{Name: "scheduledAt", Description: "Плановое время срабатывания"}}, triggerOutputDoc...),
	}
}

// ParseCron validates an expression and returns its schedule.
func ParseCron(expr string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("cron-выражение не задано")
	}
	s, err := CronParser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("некорректное cron-выражение: %v", err)
	}
	return s, nil
}

func (CronTrigger) Validate(p map[string]any) []FieldError {
	var errs []FieldError
	if expr := Str(p, "expression", ""); expr != "" {
		if _, err := ParseCron(expr); err != nil {
			errs = append(errs, FieldError{Field: "expression", Message: err.Error()})
		}
	}
	if _, err := LoadLocation(Str(p, "timezone", "")); err != nil {
		errs = append(errs, FieldError{Field: "timezone", Message: err.Error()})
	}
	return errs
}

func (CronTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	return Result{Output: triggerOutput(in, nil)}, nil
}

// ---------- Interval ----------

// IntervalTrigger fires every N units.
type IntervalTrigger struct{}

func (IntervalTrigger) Definition() Definition {
	return Definition{
		Type:        TypeIntervalTrigger,
		Name:        "Interval Trigger",
		Category:    "Triggers",
		Description: "Запускает Workflow через равные промежутки времени, когда Workflow активен.",
		Icon:        "repeat",
		KeyPrefix:   "interval",
		Trigger:     TriggerInterval,
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "every", Label: "Каждые", Kind: ParamNumber, Required: true, Default: 5, Min: ptr(1)},
			{Key: "unit", Label: "Единица", Kind: ParamSelect, Required: true, Default: "min", Options: []Option{
				{Value: "s", Label: "Seconds"}, {Value: "min", Label: "Minutes"}, {Value: "h", Label: "Hours"},
			}},
		},
		OutputDoc: append([]Field{{Name: "scheduledAt", Description: "Плановое время срабатывания"}}, triggerOutputDoc...),
	}
}

// IntervalDuration returns the configured interval.
func IntervalDuration(p map[string]any) (time.Duration, error) {
	n, ok := Num(p, "every")
	if !ok || n <= 0 {
		return 0, fmt.Errorf("интервал должен быть положительным числом")
	}
	unit := map[string]time.Duration{"s": time.Second, "min": time.Minute, "h": time.Hour}[Str(p, "unit", "min")]
	if unit == 0 {
		return 0, fmt.Errorf("неизвестная единица времени")
	}
	d := time.Duration(n * float64(unit))
	if d < MinInterval {
		return 0, fmt.Errorf("минимальный интервал — %d секунд", int(MinInterval.Seconds()))
	}
	if d > 31*24*time.Hour {
		return 0, fmt.Errorf("максимальный интервал — 31 день")
	}
	return d, nil
}

func (IntervalTrigger) Validate(p map[string]any) []FieldError {
	if _, ok := Num(p, "every"); !ok {
		return nil
	}
	if _, err := IntervalDuration(p); err != nil {
		return []FieldError{{Field: "every", Message: err.Error()}}
	}
	return nil
}

func (IntervalTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	return Result{Output: triggerOutput(in, nil)}, nil
}

// ---------- Schedule (one-time) ----------

// ScheduleLayout is the datetime format of the Schedule Trigger.
const ScheduleLayout = "2006-01-02T15:04"

// ScheduleTrigger fires once at a given date and time.
type ScheduleTrigger struct{}

func (ScheduleTrigger) Definition() Definition {
	return Definition{
		Type:        TypeScheduleTrigger,
		Name:        "Schedule Trigger",
		Category:    "Triggers",
		Description: "Однократно запускает Workflow в заданные дату и время, когда Workflow активен.",
		Icon:        "alarm-clock",
		KeyPrefix:   "schedule",
		Trigger:     TriggerSchedule,
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "at", Label: "Дата и время", Kind: ParamString, Required: true, Placeholder: "2026-12-31T09:00",
				Description: "Формат ГГГГ-ММ-ДДTЧЧ:ММ. Пропущенный из-за выключенного компьютера запуск не выполняется."},
			timezoneParam,
		},
		OutputDoc: append([]Field{{Name: "scheduledAt", Description: "Плановое время срабатывания"}}, triggerOutputDoc...),
	}
}

// ScheduleTime parses the configured moment.
func ScheduleTime(p map[string]any) (time.Time, error) {
	loc, err := LoadLocation(Str(p, "timezone", ""))
	if err != nil {
		return time.Time{}, err
	}
	raw := strings.TrimSpace(Str(p, "at", ""))
	t, err := time.ParseInLocation(ScheduleLayout, raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("дата должна быть в формате ГГГГ-ММ-ДДTЧЧ:ММ")
	}
	return t, nil
}

func (ScheduleTrigger) Validate(p map[string]any) []FieldError {
	if Str(p, "at", "") == "" {
		return nil
	}
	if _, err := ScheduleTime(p); err != nil {
		return []FieldError{{Field: "at", Message: err.Error()}}
	}
	return nil
}

func (ScheduleTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	return Result{Output: triggerOutput(in, nil)}, nil
}

// ---------- Startup ----------

// StartupTrigger fires once when the application starts.
type StartupTrigger struct{}

func (StartupTrigger) Definition() Definition {
	return Definition{
		Type:        TypeStartupTrigger,
		Name:        "Application Startup",
		Category:    "Triggers",
		Description: "Запускает Workflow при старте Veylora, когда Workflow активен.",
		Icon:        "power",
		KeyPrefix:   "startup",
		Trigger:     TriggerStartup,
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params:      []ParamSpec{},
		OutputDoc:   triggerOutputDoc,
	}
}

func (StartupTrigger) Validate(map[string]any) []FieldError { return nil }

func (StartupTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	return Result{Output: triggerOutput(in, nil)}, nil
}
