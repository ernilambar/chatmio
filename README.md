# chattu

Fast, local-first desktop client for any OpenAI-compatible API.

Chat completions use the configured chat model; thread titles use the title model and fall back to
the chat model when no title model is set.

## Features

- Per-chat model selection, changeable from the chat header.
- Searchable model picker in settings, populated from the provider's `GET /models`.
- Image and text-file attachments; unsupported binary files are rejected before sending.
- Rename and delete chats, copy messages, and automatic titles with a local fallback.
- Retry a failed turn, regenerate the last reply, and edit any user message in the composer.
- Animated thinking indicator while a reply streams; generated titles appear in the sidebar.

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
scripts/bundle.sh                quality gate, arm64 .app, zip and SHA256SUMS
scripts/dev.sh                   build and install the bundle to /Applications
scripts/release.sh               tag and push a release
.github/workflows/ci.yml         quality gate
.github/workflows/release.yml    build and publish the GitHub release
```

App identity (display name, version and copyright) lives in `wails.json` under `info`; the bundle
identifier is set in `build/darwin/Info.plist`.

## Storage

- `config.json` in the OS app data directory: base URL, chat model, title model, system prompt, API key.
- `chats.db` in the same directory: threads, messages and attachments.

API keys are stored in plain text in `config.json` for now. OS keychain support is planned.

## Development

```sh
wails dev
```

To test the installed bundle (Dock icon, app menu, LaunchServices registration):

```sh
scripts/dev.sh             # debug build
scripts/dev.sh --release   # publishable build, same as scripts/bundle.sh
```

## Build

```sh
scripts/bundle.sh
```

Runs the quality gate, regenerates and checks the Wails bindings, builds the arm64 bundle and
produces:

- `dist/Chattu.app`
- `dist/Chattu-macos-arm64.zip`
- `dist/SHA256SUMS`

## Release

Bump `info.productVersion` in `wails.json`, commit it, then:

```sh
scripts/release.sh v0.1.0
```

The tag must match `wails.json`; the script builds locally, verifies the tree is still clean,
creates the tag and pushes it. GitHub Actions then builds on an arm64 runner and publishes the zip
and checksum as a GitHub release.
