#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "usage: scripts/release.sh v0.1.0" >&2
  exit 1
fi
if [[ "$VERSION" != v* ]]; then
  VERSION="v$VERSION"
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is not clean" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then
  echo "error: tag $VERSION already exists" >&2
  exit 1
fi

VERSION="$VERSION" scripts/build-app.sh

git tag -a "$VERSION" -m "$VERSION"
git push origin "$VERSION"
echo "pushed $VERSION; GitHub Actions will publish the release"
