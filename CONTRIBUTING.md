# Contributing

Contributions are welcome. This project targets macOS on Apple silicon only; keep changes working
on arm64 and do not add Intel, universal, or non-macOS build paths.

## Prerequisites

- macOS on Apple silicon
- Go (version from `go.mod`)
- Node.js 24
- Xcode command line tools
- Wails CLI pinned to the `github.com/wailsapp/wails/v2` version in `go.mod`

## Setup

```sh
export CGO_ENABLED=1
go install "github.com/wailsapp/wails/v2/cmd/wails@$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)"
export PATH="$(go env GOPATH)/bin:$PATH"
npm --prefix frontend ci
```

No `.env` file is required. At runtime the app reads and writes `config.json` and `chats.db` in the
OS app data directory; configure the base URL, API key, and models in the in-app settings panel.

## Running

```sh
wails dev
```

To test the installed bundle (Dock icon, app menu, LaunchServices registration):

```sh
scripts/dev.sh             # debug build
scripts/dev.sh --release   # publishable build, same as scripts/bundle.sh
```

## Project layout

```
main.go                          Wails entrypoint and window options
app.go                           IPC bridge exposed to the frontend
pkg/ai                           OpenAI-compatible SSE chat client
pkg/config                       config.json load/save
pkg/db                           SQLite store and schema
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

App identity (display name, version, and copyright) lives in `wails.json` under `info`; the bundle
identifier is set in `build/darwin/Info.plist`.

## Quality gate

Run from the repository root and confirm every command exits 0 before opening a pull request:

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

The two `test -z` checks require generated bindings to be committed and all Go files formatted.

## Conventions

- `main.go` embeds `frontend/dist`, so build the frontend before any Go compile or `go vet` on a
  fresh checkout.
- `frontend/src/lib/wailsjs` is generated. Never edit it by hand; regenerate with
  `wails generate module`, normalize file modes, and commit the result. Exported methods on `App` in
  `app.go` are the frontend/backend IPC contract.
- Use Svelte 5 runes (`$state`, `$derived`, `$props`) and Tailwind utility classes. Shared UI state
  belongs in the runes-based stores under `frontend/src/lib/stores`.
- Persistence is backend-only: config through `pkg/config`, chats and messages through `pkg/db`.
  The frontend calls Wails bindings instead of touching files or SQLite.
- Keep commits focused and use a concise subject line.

## Building a bundle

```sh
scripts/bundle.sh
```

This runs the quality gate, regenerates and checks the Wails bindings, builds the arm64 bundle, and
produces:

- `dist/Chatmio.app`
- `dist/Chatmio-macos-arm64.zip`
- `dist/SHA256SUMS`

## Release

Bump `info.productVersion` in `wails.json`, commit it, then:

```sh
scripts/release.sh v0.1.0
```

The tag must match `wails.json`; the script builds locally, verifies the tree is still clean,
creates the tag, and pushes it. GitHub Actions then builds on an arm64 runner and publishes the zip
and checksum as a GitHub release.
