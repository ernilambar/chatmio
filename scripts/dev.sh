#!/usr/bin/env bash
# Build the app bundle, install it to /Applications and relaunch it.
#
#   ./scripts/dev.sh             debug build
#   ./scripts/dev.sh --release   publishable build via scripts/bundle.sh
#
# Installing the bundle is what makes bundle-only behaviour testable: the Dock
# icon, the app menu and LaunchServices registration come from the bundle, not
# from a bare binary. The copy already in /Applications is killed first, so its
# running shell is lost.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

read_info() {
    plutil -extract "$1" raw -o - "$root/wails.json" 2>/dev/null || true
}

app_name="$(read_info info.productName)"
project_name="$(read_info name)"
version="$(read_info info.productVersion)"
if [ -z "$app_name" ] || [ -z "$project_name" ] || [ -z "$version" ]; then
    printf 'error: wails.json must define name, info.productName and info.productVersion\n' >&2
    exit 1
fi

usage() {
    cat <<'EOF'
usage: scripts/dev.sh [--release]

  --release  run scripts/bundle.sh and install its verified bundle
EOF
}

release=false
for arg in "$@"; do
    case "$arg" in
        --release) release=true ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            printf 'unknown argument: %s\n\n' "$arg" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [ "$(uname -s)" != "Darwin" ]; then
    printf 'error: this app targets macOS only (this host is %s)\n' "$(uname -s)" >&2
    exit 1
fi

# hw.optional.arm64 stays 1 on Apple silicon even under Rosetta, where uname -m
# reports x86_64; checking the hardware keeps an Intel build from being tested
# as if it were a supported target.
if [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" != "1" ]; then
    printf 'error: Apple silicon required; this is an Intel Mac\n' >&2
    exit 1
fi

if $release; then
    "$root/scripts/bundle.sh"
    app="$root/dist/$app_name.app"
else
    wails_version="$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)"
    wails="${WAILS:-$(command -v wails || true)}"
    if [ -z "$wails" ]; then
        wails="$(go env GOPATH)/bin/wails"
    fi
    if [ ! -x "$wails" ]; then
        printf 'error: wails CLI not found; install with:\n' >&2
        printf '  go install github.com/wailsapp/wails/v2/cmd/wails@%s\n' "$wails_version" >&2
        exit 1
    fi

    printf '==> wails build (debug)\n'
    "$wails" build -clean -debug -platform darwin/arm64
    find frontend/src/lib/wailsjs -type f -exec chmod 644 {} +
    app="$root/build/bin/$project_name.app"
fi

if [ ! -d "$app" ]; then
    printf 'error: expected bundle not found: %s\n' "$app" >&2
    exit 1
fi

dest="/Applications/$app_name.app"
lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
if pkill -f "$dest/Contents/MacOS/"; then
    while pgrep -f "$dest/Contents/MacOS/" >/dev/null; do
        sleep 0.1
    done
fi

rm -rf "$dest"
ditto "$app" "$dest"
# Finder only notices a replaced bundle once LaunchServices forgets the old one.
"$lsregister" -f "$dest"

printf 'Installed %s (%s) to %s\n' "$app" "$version" "$dest"
open "$dest"
