# AGENTS.md

## Overview

Chattu is a local-first macOS desktop client for OpenAI-compatible chat APIs. The backend is Go
(Wails v2) and the frontend is Svelte 5 + TypeScript + Vite + Tailwind CSS 4.

Optional web tools: `pkg/webtools` exposes `web_search` / `web_fetch` backed by the Ollama web API
(`ollamaApiKey` in config). `streamReply` in `app.go` runs the OpenAI tool-call loop (max 5 rounds);
tool steps are stored on the assistant message for display only and are not replayed to the model.

## Setup

Supported platform: macOS on Apple silicon (arm64 only).

Prerequisites: Go (version from `go.mod`), Node.js 24, Xcode command line tools, and the Wails CLI
pinned to the `github.com/wailsapp/wails/v2` version in `go.mod`.

```sh
export CGO_ENABLED=1
go install "github.com/wailsapp/wails/v2/cmd/wails@$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)"
export PATH="$(go env GOPATH)/bin:$PATH"
npm --prefix frontend ci
```

No `.env` file is required. The app reads and writes `config.json` and `chats.db` in the OS app data
directory; base URL, API key, and models are configured in the in-app settings panel.

## Commands

```sh
npm --prefix frontend ci                 # install frontend dependencies
export CGO_ENABLED=1                     # required for the Go/Wails build
wails dev                                # run the app in development
npm --prefix frontend run build          # build frontend assets (embeds into the Go binary)
go build -o /dev/null .                  # build the Go app
./scripts/bundle.sh                      # full quality gate + release bundle in dist/
go test ./...                            # test
go vet ./...                             # lint
gofmt -w .                               # format Go
npm --prefix frontend run check          # typecheck Svelte/TypeScript
```

## Conventions

- `main.go` embeds `frontend/dist`, so build the frontend before any Go compile or `go vet` on a
  fresh checkout.
- Treat `frontend/src/lib/wailsjs` as generated code: never edit it by hand. Regenerate with
  `wails generate module`, normalize file modes with
  `find frontend/src/lib/wailsjs -type f -exec chmod 644 {} +`, and commit the result. The exported
  methods on `App` in `app.go` are the frontend/backend IPC contract.
- Use Svelte 5 runes (`$state`, `$derived`, `$props`) and Tailwind utility classes. Shared UI state
  belongs in the runes-based stores under `frontend/src/lib/stores`; do not introduce legacy Svelte
  stores or a second styling system.
- Persistence is backend-only: config through `pkg/config`, chats and messages through `pkg/db`.
  The frontend must call Wails bindings instead of touching files or SQLite.
- Pre-release: no config or database migrations. Change the schema in place; developers delete
  `chats.db` / `config.json` after a breaking change. Do not add backward-compatibility code.
- Keep the app macOS arm64 only. Do not add Intel, universal, or non-macOS build paths.

## Quality gate

Run from the repository root and confirm every command exits 0 before declaring a task complete:

```sh
export CGO_ENABLED=1
npm --prefix frontend ci
npm --prefix frontend run build
npm --prefix frontend run check
"$(go env GOPATH)/bin/wails" generate module
find frontend/src/lib/wailsjs -type f -exec chmod 644 {} +
test -z "$(git status --porcelain -- frontend/src/lib/wailsjs)"
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go build -o /dev/null .
```

The two `test -z` checks require generated bindings to be committed and all Go files formatted; fix
and re-run rather than skipping them. `./scripts/bundle.sh` runs the equivalent checks plus the
arm64 app bundle build when a full release artifact is needed.
