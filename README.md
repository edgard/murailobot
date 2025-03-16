# MurailoBot 🤖

[![Go Report Card](https://goreportcard.com/badge/github.com/edgard/murailobot)](https://goreportcard.com/report/github.com/edgard/murailobot)
[![License: CC0-1.0](https://img.shields.io/badge/License-CC0--1.0-lightgrey.svg)](http://creativecommons.org/publicdomain/zero/1.0/)
[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://golang.org)
[![GitHub Release](https://img.shields.io/github/v/release/edgard/murailobot)](https://github.com/edgard/murailobot/releases/latest)

A Telegram bot powered by AI models that provides intelligent responses through the Telegram messaging platform.

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

## Configuration

Minimal config.yaml example:
```yaml
# Telegram Bot Token (from BotFather)
bot_token: "your-telegram-bot-token"

# Your Telegram User ID (for admin access)
bot_admin_id: 123456789

# OpenAI API Key or compatible service key
ai_token: "your-openai-api-key"

# System instruction for the AI
ai_instruction: "You are a helpful assistant focused on providing clear and accurate responses."

# Instruction for profile generation
ai_profile_instruction: "You are a behavioral analyst with expertise in psychology, linguistics, and social dynamics.\n\nYour task is to analyze chat messages and build concise, meaningful profiles of users."
```

For a complete configuration with all options, see [config.yaml.example](config.yaml.example).

## Docker

```bash
docker pull ghcr.io/edgard/murailobot:latest
docker run -v $(pwd)/config.yaml:/app/config.yaml ghcr.io/edgard/murailobot:latest
```

## Usage

### Commands

- `/start` - Initialize bot conversation
- `/mrl_reset` - Clear chat history (admin only)
- `/mrl_analyze` - Analyze user messages and update profiles (admin only)
- `/mrl_profiles` - Show user profiles (admin only)
- `/mrl_edit_user` - Edit user profile data (admin only)

### Group Chat Usage

MurailoBot is designed to operate in Telegram group chats:

1. Add the bot to your group
2. Mention the bot (@your_bot_name) in your message to get a response
3. The bot will analyze the conversation context and respond appropriately
4. Group messages are saved for context and profile generation

## Features

- Chat with AI through Telegram group mentions
- Persistent conversation history with context preservation
- Advanced user profiling system with behavioral analysis
- Automated daily user profile updates
- Role-based access control
- Docker support for both AMD64 and ARM64 architectures
- Simple YAML configuration
- Efficient message cleanup and storage management

## User Profiling System

MurailoBot includes a sophisticated user profiling system that:

- Automatically analyzes message patterns and content to build psychological profiles
- Runs daily profile updates with data preservation mechanisms
- Tracks user metadata including display names, locations, and age ranges
- Maintains persistent profiles across conversations
- Enhances AI responses with contextual user information
- Preserves existing profile data while incrementally updating with new insights

The profiling system helps the bot provide more personalized and context-aware responses by analyzing:
- Language patterns and word choice
- Emotional expressions and communication style
- Recurring themes in communications
- Cultural references and personal details
- Group interaction dynamics

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
