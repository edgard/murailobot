# Technical Context

**Languages & Frameworks**
- Go (>=1.20), module-enabled (`go.mod`)

**Key Libraries & Dependencies**
- Telegram API: github.com/go-telegram/bot v1.14.2
- Scheduler: github.com/go-co-op/gocron/v2 v2.16.1 (upgraded from older cron library)
- Configuration: github.com/spf13/viper v1.20.1, go-playground/validator/v10 v10.26.0
- Database: modernc.org/sqlite v1.37.0, github.com/jmoiron/sqlx v1.4.0
- Migrations: github.com/golang-migrate/migrate/v4 v4.18.3
- Logging: Go `log/slog`
- AI Client: google.golang.org/genai v1.2.0
- Concurrency: golang.org/x/sync/errgroup v0.13.0

**Development Setup**
- VSCode with Go extension and auto-formatting (gofmt/slog style)
- Makefile targets: `make build`, `make run`, `make migrate`
- Example configuration file: `config.yaml.example`

**Tool Usage Patterns**
- gocron v2 for task scheduling with proper lifecycle management and error handling
- Viper for config loading and struct validation
- Slog for structured logging with adjustable levels and formats
- Sqlx for DB access and migrations via `migrate`
- Errgroup and `signal.NotifyContext` for graceful concurrency and shutdown
- Context-aware HTTP clients for image downloads with proper timeout handling
- go-telegram/bot for Telegram API integration with middleware support, including `GetMe` for runtime bot info retrieval.
- Explicit context timeouts for AI operations with appropriate durations

**Technical Constraints**
- SQLite for local persistence
- No external database dependencies
- AI integration limited to Gemini/GenAI SDK
- Must run within a Docker container or local Go environment
- Graceful shutdown requirement for all components
- Timeout constraints for long-running AI operations

**Operational Patterns**
- Component lifecycle management:
  - Clear initialization sequence in main.go
  - Graceful termination with context cancellation
  - Resource cleanup with proper error handling
- Error recovery strategies:
  - Structured logging of failures with context
  - Fallback options for non-critical errors
  - User-friendly error messages for end users
  - Detailed internal logging with query parameters and error causes
- Database transaction management:
  - Defer tx.Rollback() for safety with nil assignment after commit
  - Parameter validation before beginning transactions
  - Context timeout propagation to database operations
  - Explicit row affect checking for critical operations
  - Atomic operations for related database changes
- Security implementation:
  - Middleware-based access control for administrative commands
  - Clear separation between public and protected commands
  - Validation of user input before processing
- Performance considerations:
  - Context timeouts for long-running operations
  - Efficient database query patterns
  - Proper resource management and cleanup
  - Atomic operations for related database changes
  - Consolidated batch operations with statistical tracking
- AI Operations:
  - Explicit timeout management with reasonable durations
  - Error recovery with partial success handling
  - Statistical tracking for batch operations
  - Closure-based scope management for complex operations
