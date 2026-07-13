// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package ghfake is an in-memory implementation of the ghclient store
// interfaces for controller tests: deterministic, mutation-recording, and
// just faithful enough (label filters, search-by-label queries) to exercise
// the controllers' GitHub interactions without a server.
package ghfake
