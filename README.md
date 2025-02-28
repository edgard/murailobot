# MurailoBot 🤖

[![Go Report Card](https://goreportcard.com/badge/github.com/edgard/murailobot)](https://goreportcard.com/report/github.com/edgard/murailobot)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-1.21-blue.svg)](https://golang.org)
[![GitHub Release](https://img.shields.io/github/v/release/edgard/murailobot)](https://github.com/edgard/murailobot/releases/latest)

A Telegram bot powered by OpenAI's GPT models that provides intelligent responses through the Telegram messaging platform.

## Features

- Chat with OpenAI compatible APIs through Telegram
- Persistent conversation history
- Role-based access control
- Docker support
- Simple YAML configuration

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

### Required Settings

Minimal config.yaml example:
```yaml
ai:
  token: "sk-your-token-here"         # OpenAI API token

telegram:
  token: "your-telegram-bot-token"    # Telegram bot token
  admin_id: 123456789                 # Admin's Telegram ID
```

For a complete configuration with all options, see [config.yaml.example](config.yaml.example).

### Environment Variables

Configuration can also be provided through environment variables:

```bash
export BOT_AI_TOKEN="your-openai-token"
export BOT_TELEGRAM_TOKEN="your-telegram-token"
export BOT_TELEGRAM_ADMIN_ID="123456789"
```

Environment variables follow the pattern `BOT_SECTION_KEY` where section and key correspond to the YAML structure.

## Commands

- `/start` - Initialize bot conversation
- `/mrl <message>` - Generate AI response
- `/mrl_reset` - Clear chat history (admin only)

## License

MIT License - see [LICENSE](LICENSE) file

## Links

- [OpenAI API](https://platform.openai.com/)
- [Telegram Bot API](https://core.telegram.org/bots/api)
