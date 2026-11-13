package nodes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"veylora/internal/execution"
	"veylora/internal/workflow"
)

func testInput(nodeType string, params map[string]any, allowedDirs ...string) *Input {
	vars := execution.NewContext()
	redactor := execution.NewRedactor()
	return &Input{
		RunID: "test-run",
		Node: workflow.Node{
			ID:     "n1",
			Type:   nodeType,
			Key:    "test",
			Params: params,
		},
		Vars:     vars,
		Redactor: redactor,
		Log:      func(level, msg string, data map[string]any) {},
		Env: Env{
			AllowLocalNetwork:  true,
			MaxResponseBytes:   5 << 20,
			AllowedDirectories: allowedDirs,
		},
	}
}

func TestJSONParse(t *testing.T) {
	node := JSONParse{}
	in := testInput(TypeJSONParse, map[string]any{
		"text": `{"name": "Alice", "age": 30, "admin": true, "tags": ["a", "b"]}`,
	})
	res, err := node.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output["type"] != "object" {
		t.Errorf("want type object, got %v", res.Output["type"])
	}
	if res.Output["name"] != "Alice" {
		t.Errorf("want name Alice, got %v", res.Output["name"])
	}
	if res.Output["length"] != float64(4) {
		t.Errorf("want length 4, got %v", res.Output["length"])
	}

	// Test array
	in = testInput(TypeJSONParse, map[string]any{"text": `[1, 2, 3]`})
	res, err = node.Execute(context.Background(), in)
	if err != nil || res.Output["type"] != "array" || res.Output["length"] != float64(3) {
		t.Fatalf("array parse failed: %v, %+v", err, res.Output)
	}

	// Test invalid
	in = testInput(TypeJSONParse, map[string]any{"text": `invalid-json`})
	if _, err := node.Execute(context.Background(), in); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestJSONBuild(t *testing.T) {
	node := JSONBuild{}

	// Key-value mode
	in := testInput(TypeJSONBuild, map[string]any{
		"mode": "fields",
		"fields": []any{
			map[string]any{"key": "title", "value": "Test", "enabled": true},
			map[string]any{"key": "count", "value": 42.0, "enabled": true},
			map[string]any{"key": "disabled", "value": "skip", "enabled": false},
		},
		"pretty": false,
	})
	res, err := node.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(res.Output["json"].(string)), &parsed); err != nil {
		t.Fatalf("invalid json produced: %v", err)
	}
	if parsed["title"] != "Test" || parsed["count"] != float64(42) || parsed["disabled"] != nil {
		t.Fatalf("unexpected parsed result: %+v", parsed)
	}

	// Template mode
	in = testInput(TypeJSONBuild, map[string]any{
		"mode":     "template",
		"template": `{"id": 123, "active": true}`,
		"pretty":   true,
	})
	res, err = node.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("template build failed: %v", err)
	}
	if !strings.Contains(res.Output["json"].(string), "123") {
		t.Fatalf("want 123 in json, got %s", res.Output["json"])
	}
}

func TestRegex(t *testing.T) {
	node := Regex{}

	// Find
	in := testInput(TypeRegex, map[string]any{
		"action":  "find",
		"pattern": `user_([a-z]+)_(\d+)`,
		"text":    "hello user_alice_42 world",
	})
	res, err := node.Execute(context.Background(), in)
	if err != nil || !res.Output["matched"].(bool) || res.Output["match"] != "user_alice_42" {
		t.Fatalf("find failed: %v, %+v", err, res.Output)
	}
	groups := res.Output["groups"].(map[string]any)
	if groups["1"] != "alice" || groups["2"] != "42" {
		t.Fatalf("groups mismatch: %+v", groups)
	}

	// Replace
	in = testInput(TypeRegex, map[string]any{
		"action":      "replace",
		"pattern":     `\d+`,
		"text":        "Item 10 costs 20",
		"replaceWith": "X",
	})
	res, err = node.Execute(context.Background(), in)
	if err != nil || res.Output["result"] != "Item X costs X" {
		t.Fatalf("replace failed: %v, %+v", err, res.Output)
	}

	// Find all
	in = testInput(TypeRegex, map[string]any{
		"action":  "find_all",
		"pattern": `\w+`,
		"text":    "foo bar baz",
	})
	res, err = node.Execute(context.Background(), in)
	if err != nil || len(res.Output["matches"].([]any)) != 3 {
		t.Fatalf("find_all failed: %v, %+v", err, res.Output)
	}
}

func TestBase64(t *testing.T) {
	node := Base64{}

	// Encode
	in := testInput(TypeBase64, map[string]any{
		"action": "encode",
		"text":   "Hello, Veylora!",
	})
	res, err := node.Execute(context.Background(), in)
	if err != nil || res.Output["result"] != "SGVsbG8sIFZleWxvcmEh" {
		t.Fatalf("encode failed: %v, %+v", err, res.Output)
	}

	// Decode
	in = testInput(TypeBase64, map[string]any{
		"action": "decode",
		"text":   "SGVsbG8sIFZleWxvcmEh",
	})
	res, err = node.Execute(context.Background(), in)
	if err != nil || res.Output["result"] != "Hello, Veylora!" {
		t.Fatalf("decode failed: %v, %+v", err, res.Output)
	}
}

func TestHash(t *testing.T) {
	node := Hash{}

	// SHA256 of "hello" is 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	in := testInput(TypeHash, map[string]any{
		"algorithm": "sha256",
		"text":      "hello",
	})
	res, err := node.Execute(context.Background(), in)
	if err != nil || res.Output["hash"] != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("sha256 failed: %v, %+v", err, res.Output)
	}

	// MD5 of "hello" is 5d41402abc4b2a76b9719d911017c592
	in = testInput(TypeHash, map[string]any{
		"algorithm": "md5",
		"text":      "hello",
	})
	res, err = node.Execute(context.Background(), in)
	if err != nil || res.Output["hash"] != "5d41402abc4b2a76b9719d911017c592" {
		t.Fatalf("md5 failed: %v, %+v", err, res.Output)
	}
}

func TestDateTime(t *testing.T) {
	node := DateTime{}

	in := testInput(TypeDateTime, map[string]any{
		"input":    "2026-01-01 12:00:00",
		"format":   "2006-01-02",
		"addDays":  5.0,
		"addHours": 2.0,
	})
	res, err := node.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output["formatted"] != "2026-01-06" {
		t.Errorf("want 2026-01-06, got %v", res.Output["formatted"])
	}
	if res.Output["hour"] != float64(14) {
		t.Errorf("want hour 14, got %v", res.Output["hour"])
	}
}

func TestUUIDAndRandom(t *testing.T) {
	uuidNode := UUID{}
	res, err := uuidNode.Execute(context.Background(), testInput(TypeUUID, nil))
	if err != nil || len(res.Output["uuid"].(string)) != 36 {
		t.Fatalf("uuid failed: %v, %+v", err, res.Output)
	}

	randNode := Random{}
	// Number
	inNum := testInput(TypeRandom, map[string]any{"kind": "number", "min": 10.0, "max": 20.0})
	resNum, err := randNode.Execute(context.Background(), inNum)
	if err != nil {
		t.Fatalf("random number failed: %v", err)
	}
	n := resNum.Output["number"].(float64)
	if n < 10 || n > 20 {
		t.Fatalf("out of range: %v", n)
	}

	// String
	inStr := testInput(TypeRandom, map[string]any{"kind": "string", "length": 32.0, "charset": "hex"})
	resStr, err := randNode.Execute(context.Background(), inStr)
	if err != nil || len(resStr.Output["string"].(string)) != 32 {
		t.Fatalf("random string failed: %v, %+v", err, resStr.Output)
	}
}

func TestFilesystemNodes(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sub", "test.txt")

	// 1. Write file
	writeNode := WriteFile{}
	inWrite := testInput(TypeWriteFile, map[string]any{
		"path":       filePath,
		"content":    "Hello, Filesystem!",
		"mode":       "overwrite",
		"createDirs": true,
	}, tempDir)

	resWrite, err := writeNode.Execute(context.Background(), inWrite)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if resWrite.Output["bytesWritten"] != float64(len("Hello, Filesystem!")) {
		t.Fatalf("unexpected bytes written: %v", resWrite.Output)
	}

	// 2. Read file
	readNode := ReadFile{}
	inRead := testInput(TypeReadFile, map[string]any{
		"path":     filePath,
		"encoding": "utf-8",
	}, tempDir)

	resRead, err := readNode.Execute(context.Background(), inRead)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resRead.Output["content"] != "Hello, Filesystem!" {
		t.Fatalf("content mismatch: %v", resRead.Output["content"])
	}

	// 3. Security: Path outside allowed directory must fail
	otherDir := t.TempDir()
	forbiddenFile := filepath.Join(otherDir, "secret.txt")
	_ = os.WriteFile(forbiddenFile, []byte("secret"), 0644)

	inForbidden := testInput(TypeReadFile, map[string]any{
		"path": forbiddenFile,
	}, tempDir) // only tempDir allowed, otherDir is forbidden!

	if _, err := readNode.Execute(context.Background(), inForbidden); err == nil {
		t.Fatal("expected permission error for forbidden directory")
	}
}

func TestTelegramNode(t *testing.T) {
	node := Telegram{}
	in := testInput(TypeTelegram, map[string]any{
		"botToken": "",
		"chatId":   "123",
		"message":  "hi",
	})
	if _, err := node.Execute(context.Background(), in); err == nil {
		t.Fatal("empty token must fail")
	}
}

func TestDiscordNode(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	node := Discord{}
	in := testInput(TypeDiscord, map[string]any{
		"webhookUrl": srv.URL,
		"content":    "Hello Discord",
		"username":   "VeyloraTest",
	})

	res, err := node.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("discord send failed: %v", err)
	}
	if !res.Output["success"].(bool) || res.Output["status"] != float64(204) {
		t.Fatalf("unexpected discord output: %+v", res.Output)
	}

	var payload map[string]any
	_ = json.Unmarshal(receivedBody, &payload)
	if payload["content"] != "Hello Discord" || payload["username"] != "VeyloraTest" {
		t.Fatalf("payload mismatch: %+v", payload)
	}
}
