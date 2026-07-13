// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package sourcectrl is the source-controller's engine: it turns normalized
// findings from source plugins into GitHub issues, accumulating alerts of
// the same finding type (repository + source + primary advisory) into one
// issue for the accumulation window, and flipping aged issues to
// accumulation-complete so the remediation pipeline may pick them up.
//
// The webhook path is the fast lane; the reconcile pass re-derives
// everything from GitHub (issue labels and created_at timestamps), so missed
// deliveries, restarts, and races converge. No state lives outside GitHub.
package sourcectrl
