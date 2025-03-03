package main

// Build information variables injected at compile time through linker flags.
var (
	version = "dev"     // Semantic version of the build
	commit  = "none"    // Git commit hash
	date    = "unknown" // Build timestamp
	builtBy = "unknown" // Builder identifier
)
