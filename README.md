# chattu

Fast, local-first desktop client for any OpenAI-compatible API.

## Stack

- Wails v2 (Go + native WebView)
- Svelte 5 + TypeScript + Vite
- Tailwind CSS 4

Supported platform: **arm64 macOS only**.

## Layout

```
main.go              Wails entrypoint and window options
app.go               IPC bridge exposed to the frontend
pkg/ai               OpenAI-compatible SSE chat client
pkg/config           config.json load/save
pkg/db               SQLite store and schema
frontend/src/App.svelte          shell layout
frontend/src/lib/components      UI components
frontend/src/lib/stores          runes-based state
frontend/src/lib/wailsjs         generated Go bindings
scripts/build-app.sh             build the arm64 .app and release zip
scripts/release.sh               tag and push a release
.github/workflows/release.yml    build and publish the GitHub release
```

## Storage

- `config.json` in the OS app data directory: base URL, model, system prompt, API key.
- `chats.db` in the same directory: threads and messages.

API keys are stored in plain text in `config.json` for now. OS keychain support is planned.

## Development

```sh
wails dev
```

## Build

```sh
scripts/build-app.sh
```

Produces `build/bin/chattu.app` and `dist/chattu-<version>-darwin-arm64.zip`.

## Release

```sh
scripts/release.sh v0.1.0
```

Builds locally, creates the tag and pushes it. GitHub Actions then builds on an arm64 runner and
publishes the zip as a GitHub release.
