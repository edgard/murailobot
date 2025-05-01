# System Patterns

**Architecture & Layout**
- Entry point: `cmd/bot/main.go` initializes config, logger, DB, AI client, scheduler, handlers, tasks, retrieves bot info (`GetMe`) storing it in config, and starts the orchestrator.
- Modular packages under `internal/`:
  - `config`: Viper-based `LoadConfig` and validation
  - `logger`: `NewLogger` using Go `slog`
  - `database`: `NewDB`, migrations, models, `Store` interface + implementation
  - `gemini`: AI client interface (`Client`) and SDK wrapper
  - `telegram`: `NewTelegramBot` and `RegisterHandlers` for command routing
  - `bot`: orchestrator `Run(ctx)` using `errgroup` and scheduler integration

**Design Patterns & Practices**
- Dependency Injection: interfaces (e.g., `Store`, `Client`) passed via `HandlerDeps`/`TaskDeps`.
- Registry Pattern: centralized `registry.go` in handlers/tasks for dynamic command/task registration.
- Scheduler Architecture:
  - gocron v2 scheduler wrapped in custom `Scheduler` type
  - Time-based job scheduling with cron syntax
  - Task wrapper pattern for consistent logging and error handling
  - `Start()`/`Stop()` lifecycle methods for controlled execution
- Concurrency & Shutdown:
  - `context.Context` propagation throughout
  - Context timeouts and cancellation handling in long-running operations
  - `errgroup.Group` for concurrent goroutines
  - `signal.NotifyContext` for graceful shutdown on OS signals
  - Component lifecycle management with clean startup/shutdown
- Transaction Management:
  - Begin-commit/rollback pattern with deferred cleanup
  - Atomic operations for related database modifications
  - Proper error handling with transaction state verification
  - Nil assignment after successful commit to prevent rollback execution
  - Combined operations like DeleteAllMessagesAndProfiles for guaranteed atomicity
- Error Handling:
  - Error wrapping with `fmt.Errorf` and `%w` directive
  - Specific error messages with contextual information
  - Structured logging of errors with appropriate severity levels
  - Explicit handling of context cancellation and timeout errors
  - Systematic parameter validation before database operations
  - Statistical reporting for batch operations (processed/saved counts)
  - Partial success handling with graceful degradation
- Image Handling:
  - MIME type detection for proper content type identification
  - Size-limited file downloads with context-aware HTTP requests
  - Timeout management for potentially slow operations
- Parameter Validation:
  - Early validation with descriptive error messages
  - Nil pointer checks and boundary validation
  - Reasonable defaults and limits for pagination and query parameters
- Configuration Management: Viper for loading `config.yaml`, struct validation via `go-playground/validator`.
- Database Migrations: SQL files in `migrations/`, applied at startup via `github.com/golang-migrate/migrate`.
- Logging: structured logging with Go `slog`, configurable output format and level.
- Code Organization: clear separation of concerns, small focused packages, idiomatic Go constructors (`NewX`).
- AI Operations Management:
  - Dedicated timeout contexts for long-running AI operations
  - Consolidated error handling with specific timeout detection
  - Factory pattern for handler creation with dependency injection
  - Metrics collection during batch processing (processed/saved counts)
  - Closure-based scope management for complex operations
  - Explicit state checks before progressing to subsequent steps
  - Use of JSON schema mode for structured output (`GenerateProfiles`)
  - Dynamic system instruction header injection for context (`GenerateReply`, `GenerateImageAnalysis`)

## MentionSystemInstructionHeader

- Refined MentionSystemInstructionHeader to include only internal capability descriptions and removed styling/response-format instructions (moved to system instructions).
