// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package remedctrl is the remediation-controller's engine: it picks up
// context-enhanced findings past the accumulation window, runs the coding
// agent in an ephemeral Kubernetes Job, and performs every GitHub side
// effect on the agent's behalf — the agent pod has no GitHub credentials.
//
// The controller consumes the runner's envelope events: a classification
// event lands the report, the severity/priority/recommendation labels and
// the routing decision (dismiss the alerts and close, assign a human, or
// let the same pod continue into remediation); a remediation event pushes
// the agent's branch and opens the pull request. Merging stays human; the
// merge webhook closes the issue.
//
// Every write is driven by a fresh read of the issue's labels, so
// redeliveries, restarts, and the reconcile sweep converge on the same
// state — GitHub remains the only state store.
package remedctrl
