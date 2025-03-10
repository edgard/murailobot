package main

// Build information variables are set during compilation via ldflags.
var (
	// Version contains the current semver version from git tag.
	version = "dev"

	// commit contains the git commit hash.
	commit = "none"

	// date contains the UTC build timestamp in RFC3339 format.
	date = "unknown"

	// builtBy contains the username/system that built the binary.
	builtBy = "unknown"
)
