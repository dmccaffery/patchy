// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package jobs creates and observes the ephemeral Kubernetes Jobs that run
// agent-runner: one Job per issue attempt, deterministically named, labelled
// so orphans can be reaped, and garbage collected via TTL plus an
// owner-referenced per-Job Secret.
//
// The Job shape carries the isolation model: the short-lived GitHub token
// reaches only the init container (which clones the repository and then
// strips credentials from the clone), while the agent container gets the
// model API key and PATCHY_* configuration but no GitHub credentials at all
// — the remediation-controller owns every GitHub side effect. Results come
// back on the agent container's stdout as envelope events, followed live and
// re-read at completion as the idempotent fallback.
package jobs
