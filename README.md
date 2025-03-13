# MurailoBot 🤖

[![Go Report Card](https://goreportcard.com/badge/github.com/edgard/murailobot)](https://goreportcard.com/report/github.com/edgard/murailobot)
[![License: CC0-1.0](https://img.shields.io/badge/License-CC0--1.0-lightgrey.svg)](http://creativecommons.org/publicdomain/zero/1.0/)
[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://golang.org)
[![GitHub Release](https://img.shields.io/github/v/release/edgard/murailobot)](https://github.com/edgard/murailobot/releases/latest)

A Telegram bot powered by AI models that provides intelligent responses through the Telegram messaging platform.

## Features

- Chat with AI through Telegram
- Persistent conversation history
- User profiling system with behavioral analysis
- Role-based access control
- Docker support
- Simple YAML configuration

## Prerequisites

- Go 1.24+
- Telegram Bot Token (from @BotFather)
- AI API Key (compatible with OpenAI API format)

## Quick Start

1. Get the bot:
```bash
git clone https://github.com/edgard/murailobot.git
cd murailobot
```

2. Configure:
```bash
cp config.yaml.example config.yaml
# Edit config.yaml with your tokens and admin ID
```

3. Run:
```bash
make build
./murailobot
```

## Docker

```bash
docker pull ghcr.io/edgard/murailobot:latest
docker run -v $(pwd)/config.yaml:/app/config.yaml ghcr.io/edgard/murailobot:latest
```

## Configuration

### Required Settings

Minimal config.yaml example:
```yaml
ai:
  token: "sk-your-token-here"            # AI API token
  base_url: "https://api.openai.com/v1"  # API endpoint URL

telegram:
  token: "your-telegram-bot-token"    # Telegram bot token
  admin_id: 123456789                 # Admin's Telegram ID
```

For a complete configuration with all options, see [config.yaml.example](config.yaml.example).

### Advanced Configuration

The bot supports additional configuration options:

#### AI Options
- `model`: Specify which AI model to use (default: "gpt-4")
- `temperature`: Control response randomness (0.0-2.0)
- `timeout`: Set API call timeout duration
- `instruction`: Customize the system prompt for the AI

#### Telegram Options
- `messages`: Customize bot message templates for various interactions

#### Logging Options
- `level`: Set logging detail level (debug, info, warn, error)
- `format`: Choose log format (json, text)

### Environment Variables

Configuration can also be provided through environment variables:

```bash
export BOT_AI_TOKEN="your-ai-token"
export BOT_AI_BASE_URL="https://api.openai.com/v1"
export BOT_TELEGRAM_TOKEN="your-telegram-token"
export BOT_TELEGRAM_ADMIN_ID="123456789"
```

Environment variables follow the pattern `BOT_SECTION_KEY` where section and key correspond to the YAML structure.

## Commands

- `/start` - Initialize bot conversation
- `/mrl <message>` - Generate AI response
- `/mrl_reset` - Clear chat history (admin only)

## User Profiling System

MurailoBot includes a sophisticated user profiling system that:

- Analyzes message patterns and content to build psychological profiles
- Tracks user metadata including display names, locations, and age ranges
- Maintains persistent profiles across conversations
- Enhances AI responses with contextual user information
- Preserves existing profile data while incrementally updating with new insights

The profiling system helps the bot provide more personalized and context-aware responses by analyzing:
- Language patterns and word choice
- Emotional expressions and communication style
- Recurring themes in communications
- Cultural references and personal details

## Release Process

This project uses an automated release workflow. Here's how it works:

### Automated Releases

When code is pushed to the `main` branch, the following happens automatically:

1. The CI workflow detects changes in the codebase
2. Version is automatically bumped based on commit messages:
   - `fix:` or `fix(scope):` → patch bump
   - `feat:` or `feat(scope):` → minor bump
   - `BREAKING CHANGE:` in commit body → major bump
3. A new git tag is created with the new version
4. A GitHub release is generated with release notes
5. Binary artifacts are built for multiple platforms
6. Docker images are built and pushed to GitHub Container Registry

### For Contributors

- No need to manually create version tags or releases
- Use [Conventional Commits](https://www.conventionalcommits.org/) format for your commit messages
- The release type (patch, minor, major) is determined by your commit messages

All releases are available on the [Releases](https://github.com/edgard/murailobot/releases) page.

## License

CC0 1.0 Universal - see [LICENSE](LICENSE) file

## Links

- [OpenAI API](https://platform.openai.com/)
- [OpenRouter](https://openrouter.ai/)
- [Telegram Bot API](https://core.telegram.org/bots/api)
