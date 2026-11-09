#!/usr/bin/env bash
# Run the quality gate and build the publishable macOS bundle.
#
#   ./scripts/bundle.sh
#
# Produces the same artifacts .github/workflows/release.yml attaches to a
# release: the .app bundle, its -macos-arm64.zip archive and SHA256SUMS, all
# under dist/. Only darwin/arm64 is supported: no Intel build, no universal
# binary.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

read_info() {
    plutil -extract "$1" raw -o - "$root/wails.json" 2>/dev/null || true
}

app_name="$(read_info info.productName)"
project_name="$(read_info name)"
output_name="$(read_info outputfilename)"
version="$(read_info info.productVersion)"

if [ -z "$app_name" ]; then
    printf 'error: wails.json has no info.productName\n' >&2
    exit 1
fi
if [ -z "$project_name" ] || [ -z "$output_name" ]; then
    printf 'error: wails.json has no name or outputfilename\n' >&2
    exit 1
fi
if [ -z "$version" ]; then
    printf 'error: wails.json has no info.productVersion\n' >&2
    exit 1
fi

if [ "$(uname -s)" != "Darwin" ]; then
    printf 'error: this app targets macOS only (this host is %s)\n' "$(uname -s)" >&2
    exit 1
fi

# hw.optional.arm64 stays 1 on Apple silicon even under Rosetta, where uname -m
# reports x86_64; checking the hardware keeps an Intel build from passing as
# the supported target.
if [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" != "1" ]; then
    printf 'error: Apple silicon required; this is an Intel Mac\n' >&2
    exit 1
fi

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

printf '==> frontend\n'
(
    cd frontend
    npm ci
    # main.go embeds frontend/dist, and wails generate module compiles the main
    # package, so the frontend must exist before bindings are regenerated.
    npm run build
)

printf '==> wails bindings\n'
bindings_state() {
    git status --porcelain -- frontend/src/lib/wailsjs
    git diff -- frontend/src/lib/wailsjs
}
bindings_before="$(bindings_state)"
"$wails" generate module
# Wails writes generated files executable. Normalise them so generation does
# not dirty the tree with mode-only changes.
find frontend/src/lib/wailsjs -type f -exec chmod 644 {} +
bindings_after="$(bindings_state)"
if [ "$bindings_before" != "$bindings_after" ]; then
    printf 'error: wails bindings changed during generation; commit them and retry:\n' >&2
    git status --short -- frontend/src/lib/wailsjs >&2
    exit 1
fi

(
    cd frontend
    npm run check
)

# frontend/dist is embedded by main.go, so the frontend build must exist before
# Go can compile or vet the main package on a fresh checkout.
printf '==> gofmt\n'
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
    printf 'error: not gofmt-formatted:\n%s\n' "$unformatted" >&2
    exit 1
fi

printf '==> go vet\n'
go vet ./...

printf '==> go test\n'
go test ./...

printf '==> wails build (release)\n'
rm -rf dist
mkdir -p dist
# -s skips the second frontend build; the gate above already produced
# frontend/dist. -skipbindings reuses the bindings checked above, and -m keeps
# go.mod untouched. Production mode adds the linker's -s -w flags.
"$wails" build -clean -platform darwin/arm64 -trimpath -s -skipbindings -m

built_app="$root/build/bin/$project_name.app"
if [ ! -d "$built_app" ]; then
    printf 'error: expected bundle not found: %s\n' "$built_app" >&2
    exit 1
fi

dist_app="$root/dist/$app_name.app"
ditto "$built_app" "$dist_app"

app_arch="$(lipo -archs "$dist_app/Contents/MacOS/$output_name")"
if [ "$app_arch" != "arm64" ]; then
    printf 'error: expected arm64 app binary, got: %s\n' "$app_arch" >&2
    exit 1
fi

printf '==> package\n'
zip="$root/dist/$app_name-macos-arm64.zip"
# --norsrc --noextattr keeps AppleDouble ._* entries out of the archive; the
# signature lives in the Mach-O and CodeResources, not in an xattr.
ditto -c -k --norsrc --noextattr --keepParent "$dist_app" "$zip"
(cd "$root/dist" && shasum -a 256 "$app_name-macos-arm64.zip" >SHA256SUMS)

printf '\nbundle:  %s\narchive: %s\ndigest:  %s\nversion: %s\n' \
    "$dist_app" "$zip" "$(cat "$root/dist/SHA256SUMS")" "$version"
