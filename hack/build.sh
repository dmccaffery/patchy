#!/bin/sh
# Copyright 2026 Bitwise Media Group Ltd.
# SPDX-License-Identifier: MIT
#
# Build every patchy binary into bin/ — one controller per concern plus the
# agent runtime. Same ldflags contract as .goreleaser.yaml: version metadata
# stamped into internal/version. MODULE/VERSION/COMMIT/DATE/LDFLAGS override
# the derived defaults.
set -eu

module="${MODULE:-$(go list -m)}"
version="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
commit="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
date="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
ldflags="${LDFLAGS:--s -w \
  -X $module/internal/version.Version=$version \
  -X $module/internal/version.Commit=$commit \
  -X $module/internal/version.BuildDate=$date}"
mkdir -p bin
for dir in cmd/*/; do
  app=$(basename "$dir")
  CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "bin/$app" "./cmd/$app"
done
