// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package reconcile provides the periodic reconciliation loop every
// controller pairs with its webhook receiver. Webhooks are the fast path;
// the loop is the source of truth's safety net — it re-reads GitHub state on
// an interval so missed deliveries, controller restarts, and time-based
// transitions (the 1-hour accumulation window) all converge without any
// local state.
package reconcile
