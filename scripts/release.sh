#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

read_info() {
    plutil -extract "$1" raw -o - wails.json 2>/dev/null || true
}

version="$(read_info info.productVersion)"
if [ -z "$version" ]; then
    echo "error: wails.json has no info.productVersion" >&2
    exit 1
fi

tag="${1:-}"
if [ -z "$tag" ]; then
    echo "usage: scripts/release.sh vX.Y.Z (current version: v$version)" >&2
    exit 1
fi
if [[ "$tag" != v* ]]; then
    tag="v$tag"
fi

expected="v$version"
if [[ "$tag" != "$expected" ]]; then
    echo "error: $tag does not match wails.json info.productVersion $version (expected $expected)" >&2
    exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
    echo "error: working tree is not clean" >&2
    exit 1
fi

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
    echo "error: tag $tag already exists" >&2
    exit 1
fi

scripts/bundle.sh

# The quality gate may regenerate bindings. Tag only committed sources.
if [[ -n "$(git status --porcelain)" ]]; then
    echo "error: the build changed tracked files; commit them and retry" >&2
    exit 1
fi

git tag -a "$tag" -m "$tag"
git push origin "$tag"
echo "pushed $tag; GitHub Actions will publish the release"
