package nodes

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrFileNotFound is returned when the target file does not exist.
var ErrFileNotFound = errors.New("файл не найден")

// ErrNoAllowedDirectories is returned when the user hasn't configured any allowed folders in Settings.
var ErrNoAllowedDirectories = errors.New("в настройках не заданы разрешённые каталоги (Settings → Разрешённые каталоги)")

// ResolveAllowedFile checks if a file path is located inside one of the user's allowed directories.
// It resolves symlinks and protects against path traversal attempts ("..").
func ResolveAllowedFile(filePath string, allowedDirs []string, mustExist bool) (string, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", errors.New("путь к файлу не задан")
	}
	if !filepath.IsAbs(filePath) {
		return "", errors.New("укажите полный (абсолютный) путь к файлу")
	}
	if len(allowedDirs) == 0 {
		return "", ErrNoAllowedDirectories
	}

	cleanPath := filepath.Clean(filePath)

	if mustExist {
		realPath, err := filepath.EvalSymlinks(cleanPath)
		if err != nil {
			return "", ErrFileNotFound
		}
		info, err := os.Stat(realPath)
		if err != nil {
			return "", ErrFileNotFound
		}
		if info.IsDir() {
			return "", errors.New("указанный путь является каталогом, а не файлом")
		}

		for _, root := range allowedDirs {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			realRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(realRoot, realPath)
			if err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
				return realPath, nil
			}
		}
		return "", ErrPathNotAllowed
	}

	// For write operations where the file might not exist yet:
	// verify the directory containing the file
	dir := filepath.Dir(cleanPath)
	// Walk up existing directories to resolve symlinks safely
	cur := dir
	for {
		if cur == "" || cur == filepath.Dir(cur) {
			break
		}
		if info, err := os.Stat(cur); err == nil && info.IsDir() {
			realCur, err := filepath.EvalSymlinks(cur)
			if err == nil {
				// Check whether realCur is inside an allowed directory
				for _, root := range allowedDirs {
					root = strings.TrimSpace(root)
					if root == "" {
						continue
					}
					realRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
					if err != nil {
						continue
					}
					rel, err := filepath.Rel(realRoot, realCur)
					if err == nil && (rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))) {
						return cleanPath, nil
					}
				}
			}
			break
		}
		cur = filepath.Dir(cur)
	}

	return "", ErrPathNotAllowed
}

// ---------- Read File ----------

type ReadFile struct{}

func (ReadFile) Definition() Definition {
	return Definition{
		Type:        TypeReadFile,
		Name:        "Read File",
		Category:    "Filesystem",
		Description: "Читает содержимое файла из разрешённого каталога.",
		Icon:        "file-text",
		KeyPrefix:   "file",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "path",
				Label:       "Путь к файлу",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: `C:\Users\me\Documents\report.txt`,
				Description: "Полный путь к файлу. Должен находиться внутри разрешённых каталогов (Settings).",
			},
			{
				Key:      "encoding",
				Label:    "Кодировка",
				Kind:     ParamSelect,
				Default:  "utf-8",
				Required: true,
				Options: []Option{
					{Value: "utf-8", Label: "Текст (UTF-8)"},
					{Value: "base64", Label: "Двоичный в Base64"},
				},
			},
			{
				Key:         "maxBytes",
				Label:       "Максимальный размер, байт",
				Kind:        ParamNumber,
				Default:     5242880, // 5 MB
				Required:    true,
				Min:         ptr(1),
				Max:         ptr(52428800), // 50 MB
				Description: "Ограничение размера файла для предотвращения переполнения памяти.",
			},
		},
		OutputDoc: []Field{
			{Name: "content", Description: "Содержимое файла (текст или Base64-строка)"},
			{Name: "size", Description: "Размер файла в байтах"},
			{Name: "name", Description: "Имя файла"},
			{Name: "path", Description: "Полный путь к файлу"},
			{Name: "dir", Description: "Каталог файла"},
		},
	}
}

func (ReadFile) Validate(p map[string]any) []FieldError {
	return nil
}

func (ReadFile) Execute(_ context.Context, in *Input) (Result, error) {
	rawPath := in.Vars.ResolveString(Str(in.Node.Params, "path", ""))
	realPath, err := ResolveAllowedFile(rawPath, in.Env.AllowedDirectories, true)
	if err != nil {
		return Result{}, err
	}

	info, err := os.Stat(realPath)
	if err != nil {
		return Result{}, fmt.Errorf("ошибка чтения атрибутов файла: %w", err)
	}

	maxBytesVal, _ := Num(in.Node.Params, "maxBytes")
	maxBytes := int64(maxBytesVal)
	if maxBytes <= 0 {
		maxBytes = 5 << 20
	}
	if info.Size() > maxBytes {
		return Result{}, fmt.Errorf("размер файла (%d байт) превышает лимит %d байт", info.Size(), maxBytes)
	}

	f, err := os.Open(realPath)
	if err != nil {
		return Result{}, fmt.Errorf("не удалось открыть файл: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("ошибка чтения файла: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return Result{}, fmt.Errorf("размер файла превышает лимит %d байт", maxBytes)
	}

	encoding := Str(in.Node.Params, "encoding", "utf-8")
	var content string
	if encoding == "base64" {
		content = base64.StdEncoding.EncodeToString(data)
	} else {
		content = string(data)
	}

	out := map[string]any{
		"content": content,
		"size":    float64(len(data)),
		"name":    filepath.Base(realPath),
		"path":    realPath,
		"dir":     filepath.Dir(realPath),
	}
	return Result{Output: out}, nil
}

// ---------- Write File ----------

type WriteFile struct{}

func (WriteFile) Definition() Definition {
	return Definition{
		Type:        TypeWriteFile,
		Name:        "Write File",
		Category:    "Filesystem",
		Description: "Записывает или дописывает текст в файл в разрешённом каталоге.",
		Icon:        "file-pen",
		KeyPrefix:   "file",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "path",
				Label:       "Путь к файлу",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: `C:\Users\me\Documents\output.txt`,
				Description: "Полный путь к целевому файлу. Должен находиться внутри разрешённых каталогов.",
			},
			{
				Key:         "content",
				Label:       "Содержимое",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "Текст или {{variable}} для записи в файл",
			},
			{
				Key:      "mode",
				Label:    "Режим записи",
				Kind:     ParamSelect,
				Default:  "overwrite",
				Required: true,
				Options: []Option{
					{Value: "overwrite", Label: "Перезаписать файл (Overwrite)"},
					{Value: "append", Label: "Дописать в конец (Append)"},
				},
			},
			{
				Key:     "createDirs",
				Label:   "Создать подкаталоги, если не существуют",
				Kind:    ParamBoolean,
				Default: true,
			},
		},
		OutputDoc: []Field{
			{Name: "path", Description: "Полный путь к записанному файлу"},
			{Name: "bytesWritten", Description: "Количество записанных байт"},
			{Name: "name", Description: "Имя файла"},
		},
	}
}

func (WriteFile) Validate(p map[string]any) []FieldError {
	return nil
}

func (WriteFile) Execute(_ context.Context, in *Input) (Result, error) {
	rawPath := in.Vars.ResolveString(Str(in.Node.Params, "path", ""))
	realPath, err := ResolveAllowedFile(rawPath, in.Env.AllowedDirectories, false)
	if err != nil {
		return Result{}, err
	}

	content := in.Vars.ResolveString(Str(in.Node.Params, "content", ""))
	mode := Str(in.Node.Params, "mode", "overwrite")
	createDirs := Bool(in.Node.Params, "createDirs", true)

	dir := filepath.Dir(realPath)
	if createDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return Result{}, fmt.Errorf("не удалось создать каталоги: %w", err)
		}
	}

	flag := os.O_CREATE | os.O_WRONLY
	if mode == "append" {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(realPath, flag, 0644)
	if err != nil {
		return Result{}, fmt.Errorf("не удалось открыть файл для записи: %w", err)
	}
	defer f.Close()

	n, err := f.WriteString(content)
	if err != nil {
		return Result{}, fmt.Errorf("ошибка записи в файл: %w", err)
	}

	out := map[string]any{
		"path":         realPath,
		"bytesWritten": float64(n),
		"name":         filepath.Base(realPath),
	}
	return Result{Output: out}, nil
}
