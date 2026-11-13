package nodes

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/uuid"
)

// ---------- UUID Generator ----------

type UUID struct{}

func (UUID) Definition() Definition {
	return Definition{
		Type:        TypeUUID,
		Name:        "UUID",
		Category:    "Utilities",
		Description: "Генерирует уникальный идентификатор UUID v4.",
		Icon:        "fingerprint",
		KeyPrefix:   "uuid",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params:      []ParamSpec{},
		OutputDoc: []Field{
			{Name: "uuid", Description: "Сгенерированный UUID v4 (например, c29b9f91-7294-4b5b-9d66-0dbf53139da1)"},
		},
	}
}

func (UUID) Validate(p map[string]any) []FieldError { return nil }

func (UUID) Execute(_ context.Context, _ *Input) (Result, error) {
	id := uuid.NewString()
	return Result{Output: map[string]any{"uuid": id}}, nil
}

// ---------- Random Value Generator ----------

type Random struct{}

func (Random) Definition() Definition {
	return Definition{
		Type:        TypeRandom,
		Name:        "Random Value",
		Category:    "Utilities",
		Description: "Генерирует случайное число в диапазоне или случайную строку заданной длины.",
		Icon:        "dice",
		KeyPrefix:   "rand",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:      "kind",
				Label:    "Тип значения",
				Kind:     ParamSelect,
				Required: true,
				Default:  "number",
				Options: []Option{
					{Value: "number", Label: "Случайное число"},
					{Value: "string", Label: "Случайная строка"},
				},
			},
			{
				Key:      "min",
				Label:    "Минимум",
				Kind:     ParamNumber,
				Default:  1,
				Required: true,
				OptionalWhen: &ParamCondition{
					Param: "kind",
					In:    []string{"string"},
				},
			},
			{
				Key:      "max",
				Label:    "Максимум",
				Kind:     ParamNumber,
				Default:  100,
				Required: true,
				OptionalWhen: &ParamCondition{
					Param: "kind",
					In:    []string{"string"},
				},
			},
			{
				Key:      "length",
				Label:    "Длина строки",
				Kind:     ParamNumber,
				Default:  16,
				Required: true,
				Min:      ptr(1),
				Max:      ptr(1024),
				OptionalWhen: &ParamCondition{
					Param: "kind",
					In:    []string{"number"},
				},
			},
			{
				Key:      "charset",
				Label:    "Набор символов",
				Kind:     ParamSelect,
				Default:  "alphanumeric",
				Required: true,
				Options: []Option{
					{Value: "alphanumeric", Label: "Буквы и цифры (a-z, A-Z, 0-9)"},
					{Value: "digits", Label: "Только цифры (0-9)"},
					{Value: "hex", Label: "Шестнадцатеричные (0-9, a-f)"},
					{Value: "letters", Label: "Только буквы (a-z, A-Z)"},
				},
				OptionalWhen: &ParamCondition{
					Param: "kind",
					In:    []string{"number"},
				},
			},
		},
		OutputDoc: []Field{
			{Name: "value", Description: "Сгенерированное значение (число или строка)"},
		},
	}
}

func (Random) Validate(p map[string]any) []FieldError {
	kind := Str(p, "kind", "number")
	if kind == "number" {
		minVal, _ := Num(p, "min")
		maxVal, _ := Num(p, "max")
		if minVal > maxVal {
			return []FieldError{{Field: "min", Message: "минимум не может быть больше максимума"}}
		}
	}
	return nil
}

const (
	charsAlphaNum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charsDigits   = "0123456789"
	charsHex      = "0123456789abcdef"
	charsLetters  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

func (Random) Execute(_ context.Context, in *Input) (Result, error) {
	kind := Str(in.Node.Params, "kind", "number")

	if kind == "string" {
		lengthVal, _ := Num(in.Node.Params, "length")
		length := int(lengthVal)
		if length <= 0 {
			length = 16
		}
		if length > 1024 {
			length = 1024
		}
		charsetKind := Str(in.Node.Params, "charset", "alphanumeric")
		charset := charsAlphaNum
		switch charsetKind {
		case "digits":
			charset = charsDigits
		case "hex":
			charset = charsHex
		case "letters":
			charset = charsLetters
		}

		b := make([]byte, length)
		charsetLen := big.NewInt(int64(len(charset)))
		for i := 0; i < length; i++ {
			n, err := rand.Int(rand.Reader, charsetLen)
			if err != nil {
				return Result{}, errors.New("ошибка генерации случайной строки")
			}
			b[i] = charset[n.Int64()]
		}
		res := string(b)
		return Result{Output: map[string]any{"value": res, "string": res}}, nil
	}

	// Number
	minVal, _ := Num(in.Node.Params, "min")
	maxVal, _ := Num(in.Node.Params, "max")
	if minVal > maxVal {
		return Result{}, fmt.Errorf("минимум (%.0f) превышает максимум (%.0f)", minVal, maxVal)
	}

	diff := int64(maxVal - minVal + 1)
	if diff <= 0 {
		return Result{Output: map[string]any{"value": minVal, "number": minVal}}, nil
	}

	n, err := rand.Int(rand.Reader, big.NewInt(diff))
	if err != nil {
		return Result{}, errors.New("ошибка генерации случайного числа")
	}
	res := minVal + float64(n.Int64())
	return Result{Output: map[string]any{"value": res, "number": res}}, nil
}
