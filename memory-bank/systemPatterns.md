# System Patterns

**Architecture & Layout**

- Entry point: `cmd/bot/main.go` initializes config, logger, DB, AI client, scheduler, handlers, tasks, retrieves bot info (`GetMe`) storing it in config, and starts the orchestrator.
- Modular packages under `internal/`:
  - `config`: Viper-based `LoadConfig` and validation with comprehensive defaults
  - `logger`: `NewLogger` using Go `slog` with configurable levels and formats
  - `database`: `NewDB`, migrations, models, `Store` interface + implementation with transaction management
  - `gemini`: AI client interface (`Client`) and SDK wrapper with timeout management
  - `telegram`: `NewTelegramBot` and `RegisterHandlers` for command routing with middleware support
  - `bot`: orchestrator `Run(ctx)` using `errgroup` and scheduler integration with graceful shutdown

**Design Patterns & Practices**

- Dependency Injection: interfaces (e.g., `Store`, `Client`) passed via `HandlerDeps`/`TaskDeps`.
- Registry Pattern: centralized `registry.go` in handlers/tasks for dynamic command/task registration.
- Current Command Set:
  - Public Commands: `/start` (welcome), `/help` (usage information)
  - Protected Admin Commands: `/mrl_reset` (database reset), `/mrl_profiles` (view all profiles), `/mrl_edit_user` (edit user profiles)
  - Default Handler: Mention handler for AI interactions and image analysis
- Middleware System: AdminOnly middleware protects sensitive operations based on admin_user_id configuration
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
  - Default `slog` logger is explicitly set in `cmd/bot/main.go` after custom logger initialization.
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
  - `MentionSystemInstructionHeader` is parameterized (bot name, bot username) for dynamic bot identity in prompts.
  - `ProfileAnalyzerSystemInstruction` includes detailed guidelines for data preservation (especially existing info), trait quality (brevity, max count, no redundancy, consolidation, prioritization, simple terms, avoiding weak observations), and provides explicit examples.
- Deprecation Logging:
  - Deprecated functions (e.g., in `internal/database/store.go`) log a warning when called to track usage and encourage migration.
- Data Retrieval Specifics:
  - `GetRecentMessages` in `store.go` uses `effectiveBeforeID = ^uint(0)` (max uint) when `beforeID` is 0, to fetch all messages up to the specified limit.

## MentionSystemInstructionHeader

- Refined MentionSystemInstructionHeader to include only internal capability descriptions and removed styling/response-format instructions (moved to system instructions).
