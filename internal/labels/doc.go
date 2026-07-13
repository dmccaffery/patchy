// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package labels encodes patchy's state machine: the security-* GitHub issue
// labels that carry every finding's state, and the legal transitions between
// states. GitHub issues are the pipeline's only state store, so this package
// is the single source of truth for label names, value formats, and the
// transition table — every component parses and renders labels through it.
//
// GitHub caps label names at 50 characters, so labels carry only values that
// fit (the session label is truncated to a prefix); the full metadata always
// lives in the report comment's YAML frontmatter, which is the source of
// truth for anything a label abbreviates.
package labels
