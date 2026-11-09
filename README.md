# chattu

Fast, local-first desktop client for any OpenAI-compatible API.

## Stack

- Wails v2 (Go + native WebView)
- Svelte 5 + TypeScript + Vite
- Tailwind CSS 4

## Layout

```
main.go              Wails entrypoint and window options
app.go               IPC bridge exposed to the frontend
pkg/ai               OpenAI-compatible SSE chat client
pkg/config           config.json load/save
pkg/db               SQLite store and schema
pkg/keychain         OS credential store wrapper
frontend/src/App.svelte          shell layout
frontend/src/lib/components      UI components
frontend/src/lib/stores          runes-based state
frontend/src/lib/wailsjs         generated Go bindings
```

Data is stored under the OS app data directory (`config.json`, `chats.db`); API keys live in the OS keychain.

## Development

```sh
wails dev
```

## Build

```sh
wails build
```
