// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package version

// Version is injected at build time via -ldflags (see tasks.toml and
// .goreleaser.yaml).
//
// Version defaults to dev when no release metadata is supplied.
var Version = "dev"

// Commit is injected at build time via -ldflags (see tasks.toml and
// .goreleaser.yaml).
//
// Commit defaults to none when no release metadata is supplied.
var Commit = "none"

// BuildDate is injected at build time via -ldflags (see tasks.toml and
// .goreleaser.yaml).
//
// BuildDate defaults to unknown when no release metadata is supplied.
var BuildDate = "unknown"
