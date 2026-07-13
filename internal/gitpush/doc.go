// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package gitpush applies an agent's changeset to GitHub. The agent works in
// a network-isolated pod and hands its commits back as a git bundle; the
// controller unpacks that bundle in a scratch directory and pushes the
// branch with a short-lived, single-repository write token — the only place
// a push credential exists.
package gitpush
