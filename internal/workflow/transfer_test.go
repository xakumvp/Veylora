package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func secretDoc() Document {
	d := New()
	d.Nodes = []Node{{
		ID: "h", Type: "http.request", Name: "API", Key: "http",
		Params: map[string]any{
			"url": "https://user:pw@api.example.com/v1?token=abc123&page=1",
			"headers": []any{
				map[string]any{"key": "Authorization", "value": "Bearer real-secret"},
				map[string]any{"key": "X-Api-Key", "value": "{{env.key}}"},
				map[string]any{"key": "Accept", "value": "application/json"},
			},
			"password": "p@ss",
		},
	}}
	return d
}

func TestExportStripsSecrets(t *testing.T) {
	src := secretDoc()
	f, err := BuildExport("API", "", "test", src)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(f)
	text := string(raw)
	for _, leak := range []string{"real-secret", "abc123", "user:pw", "p@ss"} {
		if strings.Contains(text, leak) {
			t.Errorf("export contains secret %q", leak)
		}
	}
	for _, keep := range []string{"{{env.key}}", "application/json", "page=1"} {
		if !strings.Contains(text, keep) {
			t.Errorf("export lost non-secret value %q", keep)
		}
	}
	if len(f.RemovedSecrets) != 3 {
		t.Errorf("want 3 removed secrets, got %v", f.RemovedSecrets)
	}
	// The original document must be untouched.
	if src.Nodes[0].Params["password"] != "p@ss" {
		t.Fatal("StripSecrets mutated the source document")
	}
}

func TestImportRoundTrip(t *testing.T) {
	f, _ := BuildExport("API", "desc", "test", secretDoc())
	raw, _ := json.Marshal(f)
	got, err := ParseImport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "API" || len(got.Workflow.Nodes) != 1 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestImportRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"not json":     `{oops`,
		"wrong format": `{"format":"other","schemaVersion":1}`,
		"no version":   `{"format":"veylora.workflow"}`,
		"too new":      `{"format":"veylora.workflow","schemaVersion":42}`,
	}
	for name, data := range cases {
		if _, err := ParseImport([]byte(data)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := ParseImport([]byte(strings.Repeat(" ", MaxImportBytes+1))); err == nil {
		t.Error("oversized file accepted")
	}
}
