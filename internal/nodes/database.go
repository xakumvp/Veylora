package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// TypeSQLite identifies the SQLite Query node.
const TypeSQLite = "db.sqlite"

// SQLiteNode executes SQL queries (SELECT, INSERT, UPDATE, DELETE, DDL) on local SQLite databases.
type SQLiteNode struct{}

func (SQLiteNode) Definition() Definition {
	return Definition{
		Type:        TypeSQLite,
		Name:        "SQLite Query",
		Category:    "Database",
		Description: "Выполняет параметризованные SQL-запросы (SELECT, INSERT, UPDATE, DELETE) к базе данных SQLite.",
		Icon:        "database",
		KeyPrefix:   "db",
		Inputs:      []Port{{ID: "in", Kind: PortKindFlow}},
		Outputs:     []Port{{ID: "out", Kind: PortKindFlow}},
		Params: []ParamSpec{
			{
				Key:         "path",
				Label:       "Путь к файлу базы данных (.db / .sqlite)",
				Kind:        ParamString,
				Required:    true,
				Templatable: true,
				Placeholder: "C:/data/app.db",
				Description: "Полный путь к файлу SQLite. Файл должен находиться внутри разрешённых каталогов.",
			},
			{
				Key:      "mode",
				Label:    "Режим запроса",
				Kind:     ParamSelect,
				Required: true,
				Default:  "query",
				Options: []Option{
					{Value: "query", Label: "Query (SELECT — возвращает строки)"},
					{Value: "exec", Label: "Exec (INSERT, UPDATE, DELETE, DDL — модификация)"},
				},
				Description: "Query используется для чтения данных; Exec для вставки, изменения данных или структуры таблиц.",
			},
			{
				Key:         "query",
				Label:       "SQL Запрос",
				Kind:        ParamText,
				Required:    true,
				Templatable: true,
				Placeholder: "SELECT id, name, email FROM users WHERE status = ? LIMIT 50",
				Description: "SQL-выражение. Для передачи параметров используйте позиционные плейсхолдеры ?.",
			},
			{
				Key:         "params",
				Label:       "Параметры запроса (JSON-массив)",
				Kind:        ParamText,
				Templatable: true,
				Placeholder: `["active", 100]`,
				Description: "Массив аргументов для подстановки в плейсхолдеры ?: [\"значение1\", 42, true].",
			},
			{
				Key:         "readOnly",
				Label:       "Только чтение (Read-only)",
				Kind:        ParamBoolean,
				Default:     false,
				Description: "Открывает файл базы данных в режиме защиты от записи (file:...mode=ro).",
			},
		},
		OutputDoc: []Field{
			{Name: "rows", Description: "Массив строк результата (для Query)"},
			{Name: "rowCount", Description: "Количество возвращённых строк"},
			{Name: "first", Description: "Первая строка результата в виде объекта (или nil)"},
			{Name: "columns", Description: "Список названий столбцов"},
			{Name: "rowsAffected", Description: "Число изменённых строк (для Exec)"},
			{Name: "lastInsertId", Description: "ID последней вставленной строки (для Exec)"},
			{Name: "success", Description: "Признак успешного выполнения"},
		},
	}
}

func (SQLiteNode) Validate(p map[string]any) []FieldError {
	var errs []FieldError
	if strings.TrimSpace(Str(p, "path", "")) == "" {
		errs = append(errs, FieldError{Field: "path", Message: "укажите путь к файлу базы данных"})
	}
	if strings.TrimSpace(Str(p, "query", "")) == "" {
		errs = append(errs, FieldError{Field: "query", Message: "SQL-запрос не может быть пустым"})
	}
	return errs
}

func (SQLiteNode) Execute(ctx context.Context, in *Input) (Result, error) {
	p := in.Node.Params
	rawPath := in.Vars.ResolveString(Str(p, "path", ""))
	mode := strings.ToLower(Str(p, "mode", "query"))
	sqlQuery := in.Vars.ResolveString(Str(p, "query", ""))
	readOnly := Bool(p, "readOnly", false)

	if strings.TrimSpace(sqlQuery) == "" {
		return Result{}, errors.New("пустой SQL-запрос")
	}

	mustExist := (mode == "query" || readOnly)
	realPath, err := ResolveAllowedFile(rawPath, in.Env.AllowedDirectories, mustExist)
	if err != nil {
		return Result{}, fmt.Errorf("доступ к базе данных %q запрещён: %w", rawPath, err)
	}

	// Parse parameters if provided
	var args []any
	rawArgs := strings.TrimSpace(in.Vars.ResolveString(Str(p, "params", "")))
	if rawArgs != "" {
		if strings.HasPrefix(rawArgs, "[") && strings.HasSuffix(rawArgs, "]") {
			if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
				return Result{}, fmt.Errorf("ошибка парсинга параметров SQL (ожидается JSON-массив): %w", err)
			}
		} else {
			// Comma separated simple strings
			for _, part := range strings.Split(rawArgs, ",") {
				args = append(args, strings.TrimSpace(part))
			}
		}
	}

	// Prepare SQLite DSN
	dsn := realPath
	if readOnly {
		dsn = fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", filepath.ToSlash(realPath))
	} else {
		// Set busy timeout pragma to wait on lock rather than immediately failing
		if strings.Contains(dsn, "?") {
			dsn += "&_pragma=busy_timeout(5000)"
		} else {
			dsn = fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", filepath.ToSlash(realPath))
		}
	}

	in.Log(LevelDebug, fmt.Sprintf("SQLite [%s]: %s (args: %d)", mode, sqlQuery, len(args)), nil)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return Result{}, fmt.Errorf("не удалось подключиться к SQLite: %w", err)
	}
	defer db.Close()

	// 1-conn pool prevents lock race in local SQLite
	db.SetMaxOpenConns(1)

	// Ping database with context timeout
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		return Result{}, fmt.Errorf("ошибка проверки подключения к SQLite: %w", err)
	}

	switch mode {
	case "exec":
		res, err := db.ExecContext(ctx, sqlQuery, args...)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка выполнения SQL: %w", err)
		}
		rowsAffected, _ := res.RowsAffected()
		lastInsertId, _ := res.LastInsertId()

		in.Log(LevelInfo, fmt.Sprintf("SQL Exec завершён: затронуто %d строк, lastInsertId=%d", rowsAffected, lastInsertId), nil)
		return Result{
			Output: map[string]any{
				"rowsAffected": rowsAffected,
				"lastInsertId": lastInsertId,
				"success":      true,
			},
		}, nil

	default: // query
		rows, err := db.QueryContext(ctx, sqlQuery, args...)
		if err != nil {
			return Result{}, fmt.Errorf("ошибка выполнения SQL Query: %w", err)
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return Result{}, fmt.Errorf("не удалось получить столбцы: %w", err)
		}

		var results []map[string]any
		for rows.Next() {
			values := make([]any, len(cols))
			valPtrs := make([]any, len(cols))
			for i := range values {
				valPtrs[i] = &values[i]
			}

			if err := rows.Scan(valPtrs...); err != nil {
				return Result{}, fmt.Errorf("ошибка чтения строки: %w", err)
			}

			row := make(map[string]any, len(cols))
			for i, col := range cols {
				val := values[i]
				if b, ok := val.([]byte); ok {
					// Check if valid JSON object or array
					trimmed := strings.TrimSpace(string(b))
					if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
						(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
						var parsed any
						if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
							val = parsed
						} else {
							val = string(b)
						}
					} else {
						val = string(b)
					}
				}
				row[col] = val
			}
			results = append(results, row)
		}

		if err := rows.Err(); err != nil {
			return Result{}, fmt.Errorf("ошибка перебора строк результата: %w", err)
		}

		var first any
		if len(results) > 0 {
			first = results[0]
		} else {
			results = []map[string]any{}
		}

		in.Log(LevelInfo, fmt.Sprintf("SQL Query завершён: получено строк: %d", len(results)), nil)
		return Result{
			Output: map[string]any{
				"rows":     results,
				"rowCount": len(results),
				"first":    first,
				"columns":  cols,
				"success":  true,
			},
		}, nil
	}
}
