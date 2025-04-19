// Package ai provides interfaces and implementations for interacting with different AI backends.
package ai

import (
	"fmt"
	"log/slog"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
)

// NewAIClient creates and returns an AIClient based on the provided configuration.
// It acts as a factory, selecting either the OpenAI or Gemini implementation.
func NewAIClient(cfg *config.Config, storage *db.DB) (AIClient, error) {
	slog.Info("Initializing AI client", "backend", cfg.AIBackend)

	// Note: BotInfo is initially empty here. It needs to be set later via SetBotInfo.
	// Consider if AICore needs BotInfo during initialization or if it can be set later.
	// For now, initialize AICore without BotInfo, assuming SetBotInfo will be called.
	// If BotInfo is needed earlier, the factory signature or app initialization needs adjustment.
	core, err := NewAICore(storage, BotInfo{}, cfg.AIInstruction, cfg.AIProfileInstruction, cfg.AITimeout, cfg.AITemperature)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AICore: %w", err)
	}

	switch cfg.AIBackend {
	case "openai":
		client, err := newOpenAIClient(cfg, core)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
		}
		return client, nil
	case "gemini":
		client, err := newGeminiClient(cfg, core)
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini client: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unknown AI backend specified: %s", cfg.AIBackend)
	}
}
