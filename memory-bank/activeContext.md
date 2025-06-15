# Active Context

**Current Work Focus**

The core development work has been completed successfully. All major features are implemented and functioning properly:

- Automated profile analysis implementation with scheduled tasks ✅
- Configuration system improvements and validation refinements ✅
- Scheduler library migration from cron to gocron v2 for improved job management ✅
- Enhanced mention handler with improved context handling and image processing ✅
- Error handling and context timeout management improvements ✅
- Bot orchestration with clean shutdown procedures ✅
- Terminology standardization throughout the codebase (group_id → chat_id) ✅
- Atomic database operations with proper transaction handling ✅
- Administrative commands secured with appropriate middleware ✅
- Consistent error handling across all components ✅
- Profile analysis with robust error recovery ✅
- Consolidated timeout management for long-running AI operations ✅
- Dead code analysis and cleanup completed ✅

**Current Status: STABLE - Ready for Production Use**

**Recent Changes**

- Automated profile analysis implementation:
  - Created new scheduled profile_analysis task that runs daily at 2 AM
  - Moved profile analysis logic from manual /mrl_analyze command to automated scheduler
  - Implemented comprehensive timeout handling (5-minute timeout for entire operation)
  - Added proper error handling and logging for scheduled analysis operations
  - Removed manual /mrl_analyze command as profile analysis is now automated
  - Updated configuration with profile_analysis task settings
  - Cleaned up analyze-related message templates and configuration
- Configuration system improvements:
  - Refined validation approach in configuration using built-in validators
  - Removed custom validators in favor of built-in ones from go-playground/validator
  - Added comprehensive defaults for all configuration options
  - Updated config.yaml.example with detailed inline documentation
  - Clarified which configuration options are truly required vs optional with defaults
- Complete scheduler system overhaul:
  - Migrated from cron library to gocron v2
  - Improved job registration and management
  - Added graceful shutdown with proper job completion waiting
  - Enhanced logging for scheduled tasks with duration tracking
- Mention handler improvements:
  - Enhanced image handling capabilities with proper mime-type detection
  - Improved context propagation throughout the handler chain
  - Added better timeout handling for long-running operations
  - Implemented proper error reporting with user-friendly messages
- Bot orchestration enhancements:
  - Improved errgroup usage for concurrent component management
  - Enhanced shutdown sequence to ensure graceful termination
  - Added better error propagation from components to main process
- Systematic bug fixes continued:
  - Comprehensive parameter validation throughout the codebase
  - Enhanced transaction handling for database operations
  - Improved context timeout management to prevent resource leaks
  - Fixed edge cases in command and mention handling
  - Implemented atomic operations for related database changes
- Terminology standardization:
  - Renamed 'group_id' to 'chat_id' throughout the codebase for consistency with Telegram API
  - Updated database schema in migration files
  - Modified Message struct to use consistent field and column names
  - Updated all SQL queries and struct references
  - Ensured backward compatibility during the transition
- Database improvements:
  - Implemented DeleteAllMessagesAndProfiles for atomic reset operations
  - Enhanced transaction management with proper rollback on errors
  - Added comprehensive validation for all database operations
- Security improvements:
  - Added AdminOnly middleware to protect sensitive commands
  - Implemented access control for data manipulation operations
  - Ensured proper authentication for administrative functions
- Administrative command implementation:
  - /mrl_reset command for deleting all messages and profiles atomically
  - /mrl_edit_user command for profile management with field validation
  - /mrl_profiles command for viewing all stored user profiles
- Profile analysis enhancements:
  - Implemented timeout-aware context handling for AI operations
  - Added consolidated error handling logic for batch operations
  - Enhanced recovery mechanism for partial AI operation failures
  - Improved logging with operation timing and result statistics
  - Added structured error reporting for both users and logs
- Gemini Prompt Refinement:
  - Updated `MentionSystemInstructionHeader` to include a capabilities list (conversational assistant, admin commands, user profile analysis, image analysis, task scheduling, database operations), accept bot name and username as format parameters, and explicitly instruct the model _not_ to mimic the input message format (timestamp/UID prefix) in its replies.
  - Detailed `ProfileAnalyzerSystemInstruction` with guidelines for preserving existing profile data, specific quality requirements for traits (brevity, max 15-20 traits, no redundancy, aggressive consolidation, prioritization of personality traits, use of simple terms, avoidance of weak observations), and illustrative examples of good/bad trait formatting.
- Bot Info Retrieval: Implemented runtime retrieval of bot information (`GetMe`) and storage within the configuration struct.
- Removed deprecated methods `GetRecentMessagesInChat`, `DeleteAllMessages`, and `DeleteAllUserProfiles` from `internal/database/store.go`.
- Systematically analyzed all Go files (`cmd/`, `internal/`, `migrations/`) for dead code. No further dead code was identified.

**Next Steps**

**Immediate Priorities:**

- Implement comprehensive unit tests for core components (scheduler, mention handler, transaction logic, middleware, profile analysis)
- Add integration tests for bot orchestrator and component interactions
- Update documentation to reflect all recent changes and current feature set

**Development & Operations:**

- Set up CI/CD workflow (GitHub Actions) for automated build, test, and release
- Create helper CLI scripts for development tasks (seeding, resetting state, maintenance)
- Add telemetry/monitoring for AI operation performance and system health

**Scalability & Enhancement:**

- Implement circuit breaker pattern for AI service resilience
- Consider pagination for large profile datasets (`/mrl_profiles` command)
- Optimize message processing batch sizes for improved throughput
- Explore database connection pooling improvements
- Improve user feedback for administrative operations

**Future Exploration:**

- Consider alternative AI models or providers
- Evaluate database abstraction layer for future scalability
- Assess additional scheduled task opportunities

**Active Decisions & Considerations**

- Use built-in validators from go-playground/validator when available instead of custom implementations
- Provide sensible defaults for all configuration options while maintaining validation
- Use gocron v2 for all scheduled task management for better reliability
- Implement consistent context timeout handling across all long-running operations
- Provide detailed logging of scheduled task execution and performance
- Use standardized error handling patterns with proper error wrapping
- Ensure all components have proper shutdown procedures
- Maintain consistent terminology aligned with the Telegram API (chat_id instead of group_id)
- Use properly managed database transactions for related operations to ensure atomicity
- Provide detailed error information in logs while keeping user-facing messages simple
- Apply appropriate middleware to protect sensitive operations
- Standardize error handling patterns across all handlers
- Apply context timeouts to all AI operations with appropriate duration settings
- Implement partial success handling for batch operations rather than all-or-nothing
- Use statistical reporting for complex operations (processed/saved counts)
- Prefer explicit cancellation and timeout handling over generic error checks
- Ensure AI responses do not include the input message formatting prefixes (timestamp/UID).
- Enrich configuration at runtime where necessary (e.g., BotInfo from `GetMe`).
- AI Prompting: Explicitly instruct the model on desired output formatting, including what _not_ to include (e.g., input prefixes). Ensure `MentionSystemInstructionHeader` is parameterized for bot identity. Enforce strict content and formatting guidelines for `ProfileAnalyzerSystemInstruction` including data preservation and trait quality.
- The codebase is now leaner after the removal of unused database methods. The systematic check confirms no other obvious dead code.

**Important Patterns & Preferences**

- Configuration: Prefer built-in validators and provide sensible defaults for all options
- Scheduler Management: Use gocron v2's job API for registration and NewTask for execution
- Context Management: Propagate context through all operations with proper timeout handling
- Error Handling: Use structured logging with context and appropriate severity levels
- Component Lifecycle: Use errgroup for concurrent component management with proper shutdown
- Resource Management: Ensure all resources are properly released during shutdown
- Terminology: Use 'chat_id' consistently across code, database, and logs to match Telegram's terminology
- Transaction Management: Use defer tx.Rollback() pattern with explicit commit and nil assignment
- Middleware Application: Apply middleware in reverse order with the first in the slice being outermost
- Handler Organization: Use factory functions (newXHandler) for consistent initialization
- AI Operations: Implement context-aware timeouts with appropriate duration for profile analysis
- Batch Processing: Include detailed statistics (processed/saved counts) in logs and responses
- Error Recovery: Implement graceful degradation with partial success handling in batch operations
- Concurrency Management: Use proper timeout contexts for long-running operations with clear duration
- AI Prompting: Explicitly instruct the model on desired output formatting, including what _not_ to include (e.g., input prefixes). Parameterize `MentionSystemInstructionHeader` for bot identity. Apply strict guidelines for `ProfileAnalyzerSystemInstruction` regarding data preservation and trait quality (brevity, max traits, consolidation).
- Enrich configuration struct at runtime with dynamic data (e.g., BotInfo) when feasible.
- Continue maintaining clean code and removing unused components as the project evolves.

**Learnings & Project Insights**

- **Consolidated Error Handling:** Batch operations with statistical reporting (processed/saved counts) significantly improve robustness and provide valuable operational insights
- **Context Management:** Timeout management is critical for AI operations which may hang indefinitely; explicit context handling prevents resource leaks
- **Partial Success Patterns:** Graceful degradation in batch operations is preferable to all-or-nothing approaches for better user experience
- **Factory Functions:** Handler creation with consistent initialization patterns and dependency injection improves maintainability
- **AI Prompt Management:** Separating core system instructions from response formatting, parameterizing prompts, and providing detailed guidelines with examples leads to cleaner, more maintainable AI interactions
- **Dead Code Analysis:** Regular codebase reviews for obsolete components maintain project health and clarity
- **Transaction Management:** Atomic operations for related database changes with proper rollback patterns ensure data consistency
- **Middleware Patterns:** Centralized access control through middleware provides clean separation between public and protected operations
- **Scheduled Task Architecture:** gocron v2 with proper lifecycle management and task wrapper patterns provides reliable automation
- **Configuration Management:** Built-in validators with comprehensive defaults create more maintainable configuration systems
- **Component Lifecycle:** Proper startup/shutdown sequences with errgroup management ensure clean resource handling
