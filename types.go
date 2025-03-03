package main

import (
	"time"
)

// Application timeout constants define durations for various operations.
const (
	shutdownTimeout = 30 * time.Second // Timeout for graceful shutdown operations
	startupTimeout  = 15 * time.Second // Timeout for application startup operations
)

// Build information variables injected at compile time through linker flags.
var (
	version = "dev"     // Semantic version of the build
	commit  = "none"    // Git commit hash
	date    = "unknown" // Build timestamp
	builtBy = "unknown" // Builder identifier
)
