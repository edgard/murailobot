# MurailoBot 🤖

[![Go Report Card](https://goreportcard.com/badge/github.com/edgard/murailobot)](https://goreportcard.com/report/github.com/edgard/murailobot)
[![License: CC0-1.0](https://img.shields.io/badge/License-CC0--1.0-lightgrey.svg)](http://creativecommons.org/publicdomain/zero/1.0/)
[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://golang.org)
[![GitHub Release](https://img.shields.io/github/v/release/edgard/murailobot)](https://github.com/edgard/murailobot/releases/latest)

A Telegram bot powered by Google's Gemini AI models that provides intelligent responses through the Telegram messaging platform.

## Prerequisites

- Go 1.24+
- Telegram Bot Token (from @BotFather)
- Google Gemini API Key

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

## Configuration

Minimal config.yaml example:
```yaml
telegram:
  token: "your-telegram-bot-token" # Required
  admin_user_id: 123456789         # Required

gemini:
  api_key: "your-gemini-api-key"   # Required
```

For a complete configuration with all options, see [config.yaml.example](config.yaml.example).

## Docker

```bash
docker pull ghcr.io/edgard/murailobot:latest
docker run -v $(pwd)/config.yaml:/app/config.yaml ghcr.io/edgard/murailobot:latest
```

## Usage

### Commands

- `/start` - Introduction to the bot
- `/help` - Show available commands
- `/mrl_reset` - Clear chat history and user profiles (admin only)
- `/mrl_analyze` - Analyze unprocessed messages and update profiles (admin only)
- `/mrl_profiles` - Show all user profiles in the database (admin only)
- `/mrl_edit_user` - Edit user profile fields (admin only)

### Group Chat Usage

MurailoBot is designed to operate in Telegram group chats:

1. Add the bot to your group
2. Mention the bot (@your_bot_name) in your message to get a response
3. The bot will analyze the conversation context and respond appropriately
4. The bot can analyze both text and images when mentioned
5. Group messages are saved for context and profile generation

## Features

- Chat with Google's Gemini AI through Telegram group mentions
- Image analysis capabilities for photos shared in the chat
- Persistent conversation history with sophisticated context management
- Advanced user profiling system with behavioral analysis
- Scheduled database maintenance tasks
- Role-based access control for administrative commands
- Docker support for both AMD64 and ARM64 architectures
- Simple YAML configuration
- Efficient database management with SQLite

## User Profiling System

MurailoBot includes a sophisticated user profiling system that:

- Automatically analyzes message patterns and content to build psychological profiles
- Maintains user information including aliases, locations, age ranges, and personality traits
- Preserves profile data between sessions and incrementally builds understanding
- Enhances AI responses with contextual user information
- Supports manual profile editing by administrators

The profiling system gathers insights by analyzing:
- Language patterns and word choice
- Emotional expressions and communication style
- Recurring themes in communications
- Cultural references and personal details
- Group interaction dynamics

## Release Process

This project uses an automated release workflow. Here's how it works:

### Automated Releases

When code is pushed to the `master` branch, the following happens automatically:

1. The CI workflow validates the code with tests and linting
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

- [Google Gemini API](https://ai.google.dev/docs/gemini_api_overview)
- [Telegram Bot API](https://core.telegram.org/bots/api)
