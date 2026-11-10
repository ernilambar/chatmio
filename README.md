# Chatmio

Fast macOS desktop client for any OpenAI-compatible API.

## Features

- Searchable model picker with per-chat model selection.
- Streaming replies with GitHub-flavored Markdown, syntax highlighting, and copy buttons.
- Image and text-file attachments; unsupported binary files are rejected before sending.
- Search chats and messages (⌘K); rename, delete, retry, regenerate, or edit messages.
- Temporary chats (⌘⇧N) stay in memory only and are discarded when you leave the chat or quit the
  app.
- Stop a streaming reply with the stop button or Esc; received text is kept.
- Shortcuts: ⌘N new chat, ⌘⇧N temporary chat, ⌘K search, ⌘L focus composer, ⌘, settings, Esc stop.

## Requirements

- macOS on Apple silicon (arm64 only)
- An OpenAI-compatible API endpoint and key

## Install

Install with [Homebrew](https://brew.sh):

```sh
brew tap ernilambar/tap
brew trust ernilambar/tap
brew install --cask ernilambar/tap/chatmio
```

The app is ad-hoc signed, not notarized; the cask clears the quarantine flag. Upgrade with
`brew upgrade --cask chatmio`; remove settings and chat history too with
`brew uninstall --zap --cask chatmio`.

## Configuration

Open Settings (⌘,) and set the base URL, API key, chat model, and optional title model, system
prompt, context window, and generation defaults. New chats start from these defaults and can
override or clear them per chat. Thread titles use the title model, falling back to the chat model
when it is unset.

Settings and chat history are stored in the OS app data directory (`config.json` and `chats.db`).
API keys are stored in plain text in `config.json` for now; OS keychain support is planned.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, commands, project layout, the quality gate, and
release steps.

## License

[MIT](LICENSE) © Nilambar Sharma ([nilambar.net](https://nilambar.net/))
