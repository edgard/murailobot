# Project Brief

**Name:** Go Telegram Bot: AI Group Chat Assistant

**Purpose:**
Provide AI-powered interactive experiences within Telegram group chats. The bot listens for messages or commands in group contexts and responds using a general-purpose AI client to answer questions, facilitate discussions, and automate routine tasks.

**Core Requirements & Goals:**

- Modular architecture with clear separation of concerns (config, logging, database, AI client, Telegram integration, handlers, scheduler).
- Support for group chat contexts, including message routing and mention detection.
- Image analysis capabilities for photos shared in group chats.
- Reliable task scheduling with gocron v2 for maintenance and notifications.
- Dependency injection via interfaces (e.g., `AIClient`, `Store`) and constructor functions.
- Graceful startup and shutdown with `context.Context`, `errgroup`, and OS signal handling.
- Configuration management via Viper with schema validation.
- Persistent storage for user profiles, chat state, and scheduled tasks using SQLite.
- AI integration for natural language understanding, image analysis, and response generation.
- Extensible command and middleware framework for adding new features.
- Comprehensive error handling with user-friendly messages.
- Secure administrative operations with proper access control
- Atomic database operations for data consistency
- Graceful error handling and component lifecycle management

**Success Criteria:**

- Bot correctly responds in group chats when mentioned or when commands like `/start` and `/help` are used.
- Administrative commands (`/mrl_reset`, `/mrl_profiles`, `/mrl_edit_user`) are properly protected and functional.
- Successfully analyzes images shared in group chats with context-aware descriptions.
- Scheduled tasks execute reliably using gocron v2 and report status properly.
- Persistent storage of user preferences, chat settings, and message history.
- Atomic database operations maintain data consistency.
- Graceful handling of errors with appropriate user feedback.
- Component lifecycle management with proper startup and shutdown sequences.
- Clear structured logging and comprehensive error handling.
- Protected administrative operations with proper access control.
- Easy setup via Makefile, example config, and migration scripts.
