package workflow

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"veylora/internal/execution"
)

// FileFormat identifies Veylora export files.
const FileFormat = "veylora.workflow"

// MaxImportBytes caps the size of an imported file.
const MaxImportBytes = 10 << 20

// ExportFile is the portable on-disk representation of a workflow.
type ExportFile struct {
	Format        string   `json:"format"`
	SchemaVersion int      `json:"schemaVersion"`
	ExportedAt    string   `json:"exportedAt"`
	App           string   `json:"app"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Workflow      Document `json:"workflow"`
	// RemovedSecrets lists "<node name> → <field>" entries whose values were blanked.
	RemovedSecrets []string `json:"removedSecrets,omitempty"`
}

// BuildExport produces an export file with secrets stripped.
func BuildExport(name, description, appVersion string, doc Document) (ExportFile, error) {
	clean, removed, err := StripSecrets(doc)
	if err != nil {
		return ExportFile{}, err
	}
	return ExportFile{
		Format:         FileFormat,
		SchemaVersion:  SchemaVersion,
		ExportedAt:     time.Now().UTC().Format(time.RFC3339),
		App:            "Veylora " + appVersion,
		Name:           name,
		Description:    description,
		Workflow:       clean,
		RemovedSecrets: removed,
	}, nil
}

// StripSecrets returns a deep copy of doc in which sensitive header/query/param
// values and URL credentials are blanked. Values that are pure {{templates}} are kept.
func StripSecrets(doc Document) (Document, []string, error) {
	// Deep copy through JSON so the caller's document is untouched.
	raw, err := json.Marshal(doc)
	if err != nil {
		return Document{}, nil, err
	}
	var cp Document
	if err := json.Unmarshal(raw, &cp); err != nil {
		return Document{}, nil, err
	}
	cp.Normalize()

	var removed []string
	for i := range cp.Variables {
		v := &cp.Variables[i]
		if execution.IsSensitiveKey(v.Key) && v.Value != "" && !isPureTemplate(v.Value) {
			v.Value = ""
			removed = append(removed, "Переменные Workflow → "+v.Key)
		}
	}
	for i := range cp.Nodes {
		n := &cp.Nodes[i]
		for key, val := range n.Params {
			switch v := val.(type) {
			case string:
				if execution.IsSensitiveKey(key) && v != "" && !isPureTemplate(v) {
					n.Params[key] = ""
					removed = append(removed, fmt.Sprintf("%s → %s", n.Name, key))
					continue
				}
				if stripped, changed := stripURLSecrets(v); changed {
					n.Params[key] = stripped
					removed = append(removed, fmt.Sprintf("%s → %s (данные в URL)", n.Name, key))
				}
			case []any:
				for _, item := range v {
					row, ok := item.(map[string]any)
					if !ok {
						continue
					}
					k, _ := row["key"].(string)
					value, _ := row["value"].(string)
					if execution.IsSensitiveKey(k) && value != "" && !isPureTemplate(value) {
						row["value"] = ""
						removed = append(removed, fmt.Sprintf("%s → %s: %s", n.Name, key, k))
					}
				}
			}
		}
	}
	return cp, removed, nil
}

func isPureTemplate(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") && strings.Count(s, "{{") == 1
}

func stripURLSecrets(raw string) (string, bool) {
	if !strings.Contains(raw, "://") {
		return raw, false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return raw, false
	}
	changed := false
	if u.User != nil {
		u.User = nil
		changed = true
	}
	if u.RawQuery != "" && !strings.Contains(u.RawQuery, "{{") {
		q := u.Query()
		for k := range q {
			if execution.IsSensitiveKey(k) {
				q.Set(k, "")
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}
	if !changed {
		return raw, false
	}
	return u.String(), true
}

// ParseImport decodes and checks an export file. Graph validation happens in the caller.
func ParseImport(data []byte) (ExportFile, error) {
	if len(data) > MaxImportBytes {
		return ExportFile{}, fmt.Errorf("файл слишком большой (максимум %d МБ)", MaxImportBytes>>20)
	}
	var f ExportFile
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&f); err != nil {
		return ExportFile{}, fmt.Errorf("файл не является корректным JSON: %w", err)
	}
	if f.Format != FileFormat {
		return ExportFile{}, fmt.Errorf("это не файл Workflow Veylora (format = %q)", f.Format)
	}
	if f.SchemaVersion < 1 {
		return ExportFile{}, fmt.Errorf("в файле не указана версия схемы")
	}
	if f.SchemaVersion > SchemaVersion {
		return ExportFile{}, fmt.Errorf("файл создан более новой версией Veylora (схема v%d, поддерживается до v%d)", f.SchemaVersion, SchemaVersion)
	}
	if f.Workflow.SchemaVersion == 0 {
		f.Workflow.SchemaVersion = f.SchemaVersion
	}
	f.Workflow.Normalize()
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		f.Name = "Импортированный Workflow"
	}
	return f, nil
}
