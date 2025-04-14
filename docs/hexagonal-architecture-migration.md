# MurailoBot: Migration to Hexagonal Architecture with Uber's fx

## Table of Contents

- [MurailoBot: Migration to Hexagonal Architecture with Uber's fx](#murailobot-migration-to-hexagonal-architecture-with-ubers-fx)
  - [Table of Contents](#table-of-contents)
  - [Introduction](#introduction)
  - [What is Hexagonal Architecture?](#what-is-hexagonal-architecture)
  - [Why Uber's fx?](#why-ubers-fx)
  - [Migration Strategy](#migration-strategy)
  - [Proposed Project Structure](#proposed-project-structure)
  - [Example Component Design](#example-component-design)
    - [Domain Model and Service (Example)](#domain-model-and-service-example)
    - [Adapter Implementation (Example)](#adapter-implementation-example)
    - [fx Module (Example)](#fx-module-example)
  - [Example Main Application](#example-main-application)

## Introduction

This document outlines the strategy for migrating MurailoBot from its current architecture to a hexagonal architecture (also known as ports and adapters) leveraging Uber's fx dependency injection framework. This migration aims to improve code organization, testability, and maintainability.

## What is Hexagonal Architecture?

Hexagonal architecture is a software design pattern that emphasizes:

- **Domain-centric approach**: Business logic at the core, isolated from external concerns
- **Ports**: Interfaces defining how the core domain interacts with the outside world
- **Adapters**: Implementations of these interfaces for specific technologies
- **Dependency inversion**: Dependencies point inward toward the domain

Key benefits include:
- Clearer separation of concerns
- Enhanced testability
- Easier to swap out infrastructure components
- Protection of business logic from external changes

## Why Uber's fx?

Uber's fx provides:

- **Dependency injection**: Automatic wiring of components
- **Lifecycle management**: Structured startup and shutdown
- **Groups and modules**: Logical organization of components
- **Annotated constructors**: Clean and readable dependency declaration
- **Testing support**: Tools for mocking and testing components

These features align perfectly with hexagonal architecture, allowing us to:
- Cleanly separate ports from adapters
- Manage the lifecycle of all components
- Inject dependencies without tight coupling
- Group related components logically

## Migration Strategy

The migration will follow these phases:

1. **Define domain core and ports**
   - Identify the core business logic
   - Define interface contracts (ports)

2. **Create adapters**
   - Implement adapters for external systems
   - Connect adapters to ports

3. **Integrate with fx**
   - Organize components into modules
   - Define dependencies through constructor injection

4. **Refactor existing code**
   - Move from direct dependencies to interface-based interactions
   - Gradually migrate components

5. **Enhance testing**
   - Implement unit tests for domain logic
   - Create integration tests with mock adapters

## Proposed Project Structure

```
/murailobot
├── cmd/
│   └── murailobot/         # Main application entry point
│       └── main.go
├── configs/                # Configuration files
│   └── config.yaml
├── internal/               # Private application code
│   ├── domain/             # Core domain logic
│   │   ├── model/          # Domain models/entities
│   │   └── service/        # Business logic services
│   ├── port/               # Interface definitions
│   │   ├── ai/             # AI service interfaces
│   │   ├── bot/            # Bot interfaces
│   │   ├── scheduler/      # Scheduler interfaces
│   │   └── store/          # Storage interfaces
│   ├── adapter/            # Interface implementations
│   │   ├── ai/
│   │   │   └── openai/     # OpenAI implementation
│   │   ├── bot/
│   │   │   └── telegram/   # Telegram implementation
│   │   ├── scheduler/
│   │   │   └── cron/       # Cron-based scheduler
│   │   └── store/
│   │       └── sqlite/     # SQLite implementation
│   ├── common/             # Shared code
│   │   ├── config/         # Configuration loading
│   │   └── util/           # Common utilities (singular)
│   └── fx/                 # fx module definitions
│       ├── module.go       # (singular)
│       └── provider.go     # (singular)
```

## Example Component Design

### Domain Model and Service (Example)

```go
package model

// User represents a user in the system
type User struct {
    ID       string
    Username string
}

// In port/store/user.go
package store

import (
    "context"

    "github.com/edgard/murailobot/internal/domain/model"
)

// UserStore defines storage operations for users
type UserStore interface {
    Get(ctx context.Context, id string) (*model.User, error)
    Save(ctx context.Context, user *model.User) error
}

// In domain/service/user.go
package service

import (
    "context"

    "github.com/edgard/murailobot/internal/domain/model"
    "github.com/edgard/murailobot/internal/port/store"
)

// UserService defines user-related operations
type UserService interface {
    Get(ctx context.Context, id string) (*model.User, error)
    Create(ctx context.Context, username string) (*model.User, error)
}

type userService struct {
    store store.UserStore
}

// NewUserService creates a new UserService
func NewUserService(store store.UserStore) UserService {
    return &userService{
        store: store,
    }
}

func (s *userService) Get(ctx context.Context, id string) (*model.User, error) {
    return s.store.Get(ctx, id)
}
```

### Adapter Implementation (Example)

```go
// In adapter/store/sqlite/user.go
package sqlite

import (
    "context"
    "database/sql"

    "github.com/edgard/murailobot/internal/domain/model"
    "github.com/edgard/murailobot/internal/port/store"
)

type userStore struct {
    db *sql.DB
}

// NewUserStore creates a new SQLite user store
func NewUserStore(db *sql.DB) store.UserStore {
    return &userStore{
        db: db,
    }
}

func (s *userStore) Get(ctx context.Context, id string) (*model.User, error) {
    // Implementation using SQL queries
    // ...
    return &model.User{}, nil
}
```

### fx Module (Example)

```go
// In internal/fx/module.go
package fx

import (
    "database/sql"

    "go.uber.org/fx"
    "go.uber.org/zap"

    "github.com/edgard/murailobot/internal/adapter/store/sqlite"
    "github.com/edgard/murailobot/internal/domain/service"
    "github.com/edgard/murailobot/internal/port/bot"
    "github.com/edgard/murailobot/internal/port/store"
)

// Result grouping for better organization
type Stores struct {
    fx.Out

    Users  store.UserStore
    Config store.ConfigStore
}

// Parameter objects for constructor injection
type ServiceParams struct {
    fx.In

    Logger   *zap.Logger
    UserStore store.UserStore
    // Optional dependencies can be marked
    Metrics  Metrics `optional:"true"`
}

// NewStores provides all storage implementations
func NewStores(db *sql.DB, logger *zap.Logger) Stores {
    return Stores{
        Users:  sqlite.NewUserStore(db),
        Config: sqlite.NewConfigStore(db),
    }
}

// Using fx.Annotate for clearer dependency annotation
var UserModule = fx.Module("user",
    // Provide the database connection
    fx.Provide(
        fx.Annotate(
            sqlite.NewDB,
            fx.ResultTags(`name:"sqlite_db"`),
        ),
    ),

    // Store implementations
    fx.Provide(
        fx.Annotate(
            NewStores,
            fx.ParamTags(`name:"sqlite_db"`, ``),
        ),
    ),

    // Service implementations
    fx.Provide(
        fx.Annotate(
            service.NewUserService,
            fx.As(new(service.UserService)),
        ),
    ),

    // Lifecycle hooks for proper initialization and cleanup
    fx.Invoke(func(lc fx.Lifecycle, logger *zap.Logger, userStore store.UserStore) {
        lc.Append(fx.Hook{
            OnStart: func(ctx context.Context) error {
                logger.Info("initializing user store")
                return nil
            },
            OnStop: func(ctx context.Context) error {
                logger.Info("closing user store connections")
                return nil
            },
        })
    }),
)

// BotModule shows cross-module dependencies
var BotModule = fx.Module("bot",
    // Import dependencies from other modules
    fx.Provide(
        fx.Annotate(
            func(p ServiceParams) bot.Service {
                // Create bot service with injected dependencies
                return telegram.NewBotService(p.Logger, p.UserStore)
            },
            fx.As(new(bot.Service)),
        ),
    ),
)
```

## Example Main Application

```go
// In cmd/murailobot/main.go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "go.uber.org/fx"
    "go.uber.org/fx/fxevent"
    "go.uber.org/zap"

    "github.com/edgard/murailobot/internal/common/config"
    "github.com/edgard/murailobot/internal/port/bot"
    appfx "github.com/edgard/murailobot/internal/fx"
)

// Application holds our constructed dependencies
type Application struct {
    fx.In

    Logger  *zap.Logger
    BotSvc  bot.Service
    // Other dependencies
}

func main() {
    // Production-ready logger with structured logging
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // Application shutdown signals
    shutdown := make(chan os.Signal, 1)
    signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

    // Create and run the application with fx
    app := fx.New(
        // Global logger configuration
        fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
            return &fxevent.ZapLogger{Logger: log}
        }),

        // Supply pre-constructed dependencies
        fx.Supply(logger),

        // Load configuration
        fx.Provide(
            config.NewConfig,
        ),

        // Include all application modules
        appfx.UserModule,
        appfx.BotModule,
        appfx.AIModule,
        appfx.SchedulerModule,

        // Invoke application start
        fx.Invoke(runApp),
    )

    // Start the application in the background
    startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
    defer cancel()

    if err := app.Start(startCtx); err != nil {
        logger.Fatal("failed to start application", zap.Error(err))
    }

    // Wait for shutdown signal
    <-shutdown
    logger.Info("shutdown signal received")

    // Graceful shutdown
    stopCtx, cancel := context.WithTimeout(context.Background(), app.StopTimeout())
    defer cancel()

    if err := app.Stop(stopCtx); err != nil {
        logger.Fatal("failed to stop application", zap.Error(err))
    }
}

// runApp initializes and runs the application
func runApp(lc fx.Lifecycle, app Application) {
    lc.Append(fx.Hook{
        OnStart: func(ctx context.Context) error {
            app.Logger.Info("starting MurailoBot")
            return app.BotSvc.Start(ctx)
        },
        OnStop: func(ctx context.Context) error {
            app.Logger.Info("stopping MurailoBot")
            return app.BotSvc.Stop(ctx)
        },
    })
}
```
