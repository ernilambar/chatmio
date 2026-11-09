# chattu

Fast, local-first desktop client for any OpenAI-compatible API.

Chat completions use the configured chat model; thread titles use the title model and fall back to
the chat model when it is unset or returns no title.

## Features

- Per-chat model selection and a searchable model picker populated from the provider's `GET /models`.
- Streaming replies rendered as GitHub-flavored Markdown, with highlighted code and copy buttons.
- Image and text-file attachments; unsupported binary files are rejected before sending.
- Search chats and messages (⌘K), rename or delete chats, and retry, regenerate, or edit messages.
- Temporary chats (⌘⇧N) live only in memory: messages and attachments are never written to
  `chats.db`, and the chat is discarded when you leave it or quit the app.
- Stop a streaming reply with the stop button or Esc; received text is kept.
- Shortcuts: ⌘N new chat, ⌘⇧N temporary chat, ⌘K search, ⌘L focus composer, ⌘, settings, Esc stop.

## Requirements

- macOS on Apple silicon (arm64 only)
- An OpenAI-compatible API endpoint and key

## Install

Download `Chattu-macos-arm64.zip` from the latest release, then:

```sh
unzip Chattu-macos-arm64.zip
xattr -dr com.apple.quarantine Chattu.app
mv Chattu.app /Applications/
open /Applications/Chattu.app
```

The app is ad-hoc signed, so removing the quarantine attribute is required after downloading.

## Configuration

Open Settings (⌘,) and set the base URL, API key, chat model, and optional title model, system
prompt, context window, and generation defaults (temperature, top P, max tokens, frequency
penalty). Defaults are copied into new chats; each chat can override or clear them. Settings and
chat history are stored in the OS app data directory:

- `config.json`: base URL, models, system prompt, API key, and generation defaults.
- `chats.db`: threads, messages, and attachments.

API keys are stored in plain text in `config.json` for now; OS keychain support is planned.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, commands, project layout, the quality gate, and
release steps.

```sh
wails dev
```

## License

[MIT](LICENSE) © Nilambar Sharma ([nilambar.net](https://nilambar.net/))
