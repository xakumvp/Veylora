package nodes

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"veylora/internal/workflow"
)

func TestSQLiteNode_ExecAndQuery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	env := Env{AllowedDirectories: []string{tmpDir}}

	node := SQLiteNode{}

	// 1. Create table (exec)
	inCreate, _ := input(workflow.Node{
		ID:   "sql-create",
		Type: TypeSQLite,
		Params: map[string]any{
			"path":  dbPath,
			"mode":  "exec",
			"query": "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, email TEXT, active BOOLEAN, meta JSON)",
		},
	}, env)

	res, err := node.Execute(context.Background(), inCreate)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	if res.Output["success"] != true {
		t.Fatalf("expected success: true, got %v", res.Output)
	}

	// 2. Insert records (exec with params)
	inInsert, _ := input(workflow.Node{
		ID:   "sql-insert",
		Type: TypeSQLite,
		Params: map[string]any{
			"path":   dbPath,
			"mode":   "exec",
			"query":  "INSERT INTO users (name, email, active, meta) VALUES (?, ?, ?, ?)",
			"params": `["Alice", "alice@example.com", true, "{\"role\":\"admin\"}"]`,
		},
	}, env)

	resInsert, err := node.Execute(context.Background(), inInsert)
	if err != nil {
		t.Fatalf("insert record: %v", err)
	}
	if resInsert.Output["lastInsertId"].(int64) != 1 {
		t.Fatalf("expected lastInsertId=1, got %v", resInsert.Output["lastInsertId"])
	}
	if resInsert.Output["rowsAffected"].(int64) != 1 {
		t.Fatalf("expected rowsAffected=1, got %v", resInsert.Output["rowsAffected"])
	}

	// 3. Query records (SELECT)
	inQuery, _ := input(workflow.Node{
		ID:   "sql-query",
		Type: TypeSQLite,
		Params: map[string]any{
			"path":   dbPath,
			"mode":   "query",
			"query":  "SELECT id, name, email, active, meta FROM users WHERE active = ?",
			"params": `[1]`,
		},
	}, env)

	resQuery, err := node.Execute(context.Background(), inQuery)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resQuery.Output["rowCount"].(int) != 1 {
		t.Fatalf("expected rowCount=1, got %v", resQuery.Output["rowCount"])
	}
	rows := resQuery.Output["rows"].([]map[string]any)
	if rows[0]["name"] != "Alice" || rows[0]["email"] != "alice@example.com" {
		t.Fatalf("unexpected row data: %+v", rows[0])
	}

	// 4. Read-only mode
	inRo, _ := input(workflow.Node{
		ID:   "sql-ro",
		Type: TypeSQLite,
		Params: map[string]any{
			"path":     dbPath,
			"mode":     "query",
			"query":    "SELECT count(*) as cnt FROM users",
			"readOnly": true,
		},
	}, env)

	resRo, err := node.Execute(context.Background(), inRo)
	if err != nil {
		t.Fatalf("read-only query: %v", err)
	}
	roRows := resRo.Output["rows"].([]map[string]any)
	if roRows[0]["cnt"].(int64) != 1 {
		t.Fatalf("expected count 1, got %v", roRows[0]["cnt"])
	}
}

func TestSQLiteNode_SecurityFolderCheck(t *testing.T) {
	tmpDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.db")
	_ = os.WriteFile(outsideFile, []byte(""), 0o600)

	node := SQLiteNode{}
	env := Env{AllowedDirectories: []string{tmpDir}}

	in, _ := input(workflow.Node{
		ID:   "sql-unsafe",
		Type: TypeSQLite,
		Params: map[string]any{
			"path":  outsideFile,
			"mode":  "query",
			"query": "SELECT 1",
		},
	}, env)

	_, err := node.Execute(context.Background(), in)
	if err == nil {
		t.Fatal("expected error when accessing database outside allowed directories, got nil")
	}
}

func TestTCPCheckNode(t *testing.T) {
	// Start a local TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	// 1. Localhost allowed
	node := TCPCheckNode{}
	envAllowed := Env{AllowLocalNetwork: true}

	inAllowed, _ := input(workflow.Node{
		ID:   "tcp-1",
		Type: TypeTCPCheck,
		Params: map[string]any{
			"host":    "127.0.0.1",
			"port":    port,
			"timeout": 1000,
		},
	}, envAllowed)

	res, err := node.Execute(context.Background(), inAllowed)
	if err != nil {
		t.Fatalf("tcp check allowed: %v", err)
	}
	if res.Output["alive"] != true {
		t.Fatalf("expected alive: true, got %v", res.Output)
	}

	// 2. Localhost blocked
	envBlocked := Env{AllowLocalNetwork: false}
	inBlocked, _ := input(workflow.Node{
		ID:   "tcp-2",
		Type: TypeTCPCheck,
		Params: map[string]any{
			"host": "127.0.0.1",
			"port": port,
		},
	}, envBlocked)

	_, errBlocked := node.Execute(context.Background(), inBlocked)
	if errBlocked != ErrLocalNetworkBlocked {
		t.Fatalf("expected ErrLocalNetworkBlocked, got %v", errBlocked)
	}
}

func TestURLParseNode(t *testing.T) {
	node := URLParseNode{}
	in, _ := input(workflow.Node{
		ID:   "url-1",
		Type: TypeURLParse,
		Params: map[string]any{
			"url": "https://admin:secret@api.example.com:8443/v1/users?status=active&tag=go#section2",
		},
	}, Env{})

	res, err := node.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}
	out := res.Output
	if out["protocol"] != "https" {
		t.Fatalf("expected https, got %v", out["protocol"])
	}
	if out["hostname"] != "api.example.com" {
		t.Fatalf("expected api.example.com, got %v", out["hostname"])
	}
	if out["port"] != "8443" {
		t.Fatalf("expected 8443, got %v", out["port"])
	}
	if out["pathname"] != "/v1/users" {
		t.Fatalf("expected /v1/users, got %v", out["pathname"])
	}
	if out["hash"] != "#section2" {
		t.Fatalf("expected #section2, got %v", out["hash"])
	}
	if out["username"] != "admin" {
		t.Fatalf("expected admin, got %v", out["username"])
	}
	query := out["query"].(map[string]any)
	if query["status"] != "active" || query["tag"] != "go" {
		t.Fatalf("unexpected query params: %+v", query)
	}
}

func TestEmailNode_Validation(t *testing.T) {
	node := EmailNode{}
	errs := node.Validate(map[string]any{})
	if len(errs) != 4 {
		t.Fatalf("expected 4 validation errors for empty email params, got %d", len(errs))
	}

	// Local network check
	envBlocked := Env{AllowLocalNetwork: false}
	inBlocked, _ := input(workflow.Node{
		ID:   "email-local",
		Type: TypeEmail,
		Params: map[string]any{
			"smtpHost": "127.0.0.1",
			"smtpPort": 25,
			"from":     "test@local",
			"to":       "user@local",
			"subject":  "Test",
			"body":     "Hello",
		},
	}, envBlocked)

	_, err := node.Execute(context.Background(), inBlocked)
	if err != ErrLocalNetworkBlocked {
		t.Fatalf("expected ErrLocalNetworkBlocked, got %v", err)
	}
}
