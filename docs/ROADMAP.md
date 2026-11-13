# Veylora — Roadmap

Veylora — локальная визуальная платформа автоматизации с открытым исходным кодом.
Основная ценность: удобный Node Editor, быстрый Go Engine и расширяемая система интеграций.

Главный принцип: **Everything is a Workflow** — любая автоматизация описывается графом Nodes и Connections.

## Принципы

- **Local First** — пользователь владеет своими Workflow и данными.
- **Visual First** — основной способ создания автоматизаций — визуальный редактор.
- **Extensible** — новые интеграции добавляются плагинами.
- **Developer Friendly** — простая архитектура, документация, открытый код.
- **Reliable** — ошибки, отмена и повторные запуски обрабатываются предсказуемо.
- **Secure by Default** — опасные операции выполняются только с явного разрешения.
- **Cross Platform** — одна модель Workflow для Windows, Linux и macOS.

## Целевые режимы запуска

1. Desktop Application — основное приложение (Windows / Linux / macOS).
2. Portable Mode — запуск без установки.
3. Headless Mode — движок без GUI (`veylora server`, `veylora workflow run …`).
4. Docker — серверный движок.

## Фазы

Легенда: ✅ готово · 🚧 в работе · ⬜ запланировано

### Phase 1 — Foundation ✅ (v0.1)
- ✅ Desktop-архитектура (Wails + React), модель Workflow, SQLite с миграциями.
- ✅ Редактор: drag & drop, pan/zoom, multi-select, minimap, snap to grid, Undo/Redo, автосохранение.
- ✅ Nodes: Manual Trigger, HTTP Request, Condition, Delay, Logger.
- ✅ Валидация графа (циклы, порты, параметры), live-статусы, история запусков.
- ✅ Импорт/экспорт JSON без секретов, защита от SSRF (локальная сеть по умолчанию закрыта).

### Phase 2 — Core Automation 🚧 (v0.2)
- ✅ Расширенные переменные: `{{execution.id}}`, `{{workflow.name}}`, `{{variables.x}}`, `{{env.NAME}}` (по белому списку).
- ✅ Scheduler: Cron, Interval, однократное расписание, часовые пояса, восстановление после перезапуска.
- ✅ Триггеры: File Watcher (только разрешённые каталоги), Application Startup.
- ✅ Обработка ошибок: Retry с exponential backoff, Node Timeout, Workflow Timeout, Continue On Error, Error Output, Stop Workflow.
- ✅ Несколько одновременных запусков с лимитом и защитой от дублей.
- ✅ Фильтр истории по статусу, повторный запуск.

### Phase 3 — Advanced Editor 🚧
- ✅ Auto Layout, контекстное меню, Copy/Paste (с сохранением связей и уникальными ключами переменных).
- ✅ Встроенные шаблоны сценариев (NewWorkflowDialog), поиск по Workflow и нодам (Command Palette Ctrl+K/Ctrl+F).
- ✅ Версионирование Workflow: история изменений, автоматические снимки при сохранении, визуальный diff, восстановление (откат), экспорт любой версии.
- ✅ Subflows (Execute Workflow): ограничение глубины вызовов (до 5 уровней), защита от рекурсивных циклов, сквозные логи и проброс входных/выходных данных.
- ⬜ Группы нод на холсте, Variable Picker с автодополнением в Inspector.

### Phase 4 — Integrations & Nodes 🚧
- ✅ Обработка данных: JSON Parser, JSON Builder, Regex (поиск/замена/группы), Date / Time (форматирование, смещения), Hash Generator (MD5, SHA-1, SHA-256, SHA-512), Base64.
- ✅ Утилиты: UUID v4, Random Value (числа в диапазоне, строки с наборами символов).
- ✅ Уведомления: Telegram (Bot API), Discord (Webhooks).
- ✅ Файловая система: Read File, Write File (с контролем разрешённых каталогов и защитой от Path Traversal).
- ⬜ Встроенный Webhook Server (HTTP-сервер для триггеров по внешним вебхукам).
- ⬜ Базы данных: SQLite, PostgreSQL, MySQL, Redis.
- ⬜ Email (SMTP), Desktop Notifications.
- ⬜ Сетевые утилиты: HTTP Download/Upload, WebSocket, Ping, TCP Check, DNS Lookup.

### Phase 5 — Plugin Ecosystem ⬜
- Плагины как отдельные процессы с RPC, Manifest, разрешения, подпись пакетов.
- Plugin Manager GUI, Plugin SDK, примеры (HTTP, Telegram, Custom Trigger).

### Phase 6 — Professional Features ⬜
- Dashboard с графиками, Credential Manager с шифрованием (системное хранилище секретов).
- Параллельное выполнение независимых веток (Worker Pool), расширенные логи.
- Перезапуск с определённого Node.
- Light Theme, локализация EN/RU.

### Phase 7 — Server Mode ⬜
- Headless Engine + CLI, локальный REST API (`/api/v1/...`) с SSE, аутентификацией и rate limiting.
- Docker, PostgreSQL.

### Phase 8 — Public Release ⬜
- Security Audit, Benchmark Suite, оптимизация.
- Кросс-платформенные сборки (Windows/Linux/macOS, AMD64/ARM64), checksums, release notes.
- Документация: INSTALLATION, GETTING_STARTED, WORKFLOW_GUIDE, NODE_DEVELOPMENT, PLUGIN_SDK, API_REFERENCE, SECURITY, CONTRIBUTING.

## Критерии полноценного продукта

1. Сложные Workflow создаются визуально.
2. Engine поддерживает последовательное и параллельное выполнение.
3. Работают все основные категории Nodes.
4. Поддерживаются расписания и события.
5. Работает система плагинов, пользователь может ставить свои расширения.
6. Безопасное хранение Credentials.
7. История выполнений и диагностика.
8. Subflows.
9. Импорт и экспорт.
10. API и Headless Mode.
11. Windows, Linux и macOS.
12. Полная документация, автоматизированные тесты и CI/CD.
