# MurailoBot 🤖

[![Go Report Card](https://goreportcard.com/badge/github.com/edgard/murailobot)](https://goreportcard.com/report/github.com/edgard/murailobot)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-1.21-blue.svg)](https://golang.org)
[![GitHub Release](https://img.shields.io/github/v/release/edgard/murailobot)](https://github.com/edgard/murailobot/releases/latest)

A Telegram bot powered by AI that provides intelligent responses through the Telegram messaging platform.

## Features

- Advanced AI integration using OpenAI's GPT models
- Telegram commands: /start, /mrl, /mrl_reset
- User access control with allowed/blocked lists
- SQLite for data persistence with optimized settings
- Docker support
- Smart defaults for all non-essential settings
- Comprehensive configuration validation
- Chat history management
- Configurable response parameters
- Robust error handling and logging

## Prerequisites

- Go 1.21+
- Telegram Bot Token (from @BotFather)
- OpenAI API Key

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

The bot uses smart defaults for all non-essential settings, requiring only three critical configuration values to get started.

### Required Configuration

Minimal config.yaml example with only required fields:
```yaml
ai:
  token: "your-openai-api-token"  # Get from platform.openai.com/api-keys

telegram:
  token: "your-telegram-bot-token"  # Get from @BotFather
  admin_id: 123456789              # Get from @userinfobot
```

### Environment Variables

The required configuration can also be set via environment variables:

```bash
# AI Configuration
export BOT_AI_TOKEN="your-openai-api-token"

# Telegram Configuration
export BOT_TELEGRAM_TOKEN="your-telegram-bot-token"
export BOT_TELEGRAM_ADMIN_ID="123456789"
```

For all available configuration options, their default values, and detailed descriptions, see the annotated `config.yaml.example` file.

## Commands

- `/start` - Start conversation with the bot
- `/mrl <message>` - Generate AI response
- `/mrl_reset` - Reset chat history (admin only)

## License

MIT License - see [LICENSE](LICENSE) file

## Links

- [Telegram Bot API](https://core.telegram.org/bots/api)
- [OpenAI API](https://platform.openai.com/)
