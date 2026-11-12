package nodes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// File trigger events.
const (
	FileCreated  = "created"
	FileModified = "modified"
	FileDeleted  = "deleted"
)

// ErrPathNotAllowed is returned when a path lies outside the allowed directories.
var ErrPathNotAllowed = errors.New("каталог не входит в список разрешённых (Settings → Разрешённые каталоги)")

// FileTrigger fires when files change in a watched directory.
type FileTrigger struct{}

func (FileTrigger) Definition() Definition {
	return Definition{
		Type:        TypeFileTrigger,
		Name:        "File Watcher",
		Category:    "Triggers",
		Description: "Запускает Workflow при создании, изменении или удалении файлов в каталоге, когда Workflow активен.",
		Icon:        "folder-search",
		KeyPrefix:   "file",
		Trigger:     TriggerFile,
		Inputs:      []Port{},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{Key: "directory", Label: "Каталог", Kind: ParamString, Required: true, Placeholder: `C:\Users\me\Downloads`,
				Description: "Должен находиться внутри одного из разрешённых каталогов (Settings)."},
			{Key: "pattern", Label: "Маска файлов", Kind: ParamString, Default: "*", Placeholder: "*.pdf"},
			{Key: "onCreate", Label: "Создание файла", Kind: ParamBoolean, Default: true},
			{Key: "onModify", Label: "Изменение файла", Kind: ParamBoolean, Default: false},
			{Key: "onDelete", Label: "Удаление файла", Kind: ParamBoolean, Default: false},
			{Key: "recursive", Label: "Включая подкаталоги", Kind: ParamBoolean, Default: false},
		},
		OutputDoc: append([]Field{
			{Name: "event", Description: "created / modified / deleted"},
			{Name: "path", Description: "Полный путь к файлу"},
			{Name: "name", Description: "Имя файла"},
			{Name: "directory", Description: "Каталог файла"},
		}, triggerOutputDoc...),
	}
}

func (FileTrigger) Validate(p map[string]any) []FieldError {
	var errs []FieldError
	if pat := Str(p, "pattern", "*"); pat != "" {
		if _, err := filepath.Match(pat, "probe"); err != nil {
			errs = append(errs, FieldError{Field: "pattern", Message: "некорректная маска файлов"})
		}
	}
	if !Bool(p, "onCreate", true) && !Bool(p, "onModify", false) && !Bool(p, "onDelete", false) {
		errs = append(errs, FieldError{Field: "onCreate", Message: "выберите хотя бы одно событие"})
	}
	return errs
}

func (FileTrigger) Execute(_ context.Context, in *Input) (Result, error) {
	extra := map[string]any{}
	if in.Event == nil || in.Event.Source != TriggerFile {
		// Manual test run: describe the watched directory instead of a real event.
		dir := Str(in.Node.Params, "directory", "")
		extra = map[string]any{"event": "test", "path": dir, "name": "", "directory": dir}
	}
	return Result{Output: triggerOutput(in, extra)}, nil
}

// ResolveAllowedDir returns the absolute, symlink-resolved directory if it lies
// within one of the allowed roots. It protects against path traversal ("..")
// and symlinks pointing outside the allowed area.
func ResolveAllowedDir(dir string, allowed []string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", fmt.Errorf("каталог не задан")
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("укажите полный путь к каталогу")
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return "", fmt.Errorf("каталог не найден")
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("путь не является каталогом")
	}
	for _, root := range allowed {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		rootReal, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil {
			continue
		}
		if within(rootReal, real) {
			return real, nil
		}
	}
	return "", ErrPathNotAllowed
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
