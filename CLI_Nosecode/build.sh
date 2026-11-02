#!/usr/bin/env sh
# Compila y deja el binario listo en dist/.
set -eu

cd "$(dirname "$0")"
mkdir -p dist

VERSION="${VERSION:-0.4.0}"
LDFLAGS="-s -w -X nosecode/internal/version.Version=${VERSION} -X nosecode/internal/version.Commit=build"

echo "Nosecode: compilando..."
go build -trimpath -ldflags "$LDFLAGS" -o dist/nosecode ./cmd/nosecode

echo "Listo: dist/nosecode"
