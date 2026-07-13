// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package contextctrl is the context-controller's engine: it reacts to
// newly opened security-finding issues, runs the enhancer chain (CMDB
// ownership, infrastructure context — pkg/enhance plugins), posts each
// contribution as an attributed comment carrying a machine-readable
// enrichment block, and advances the issue to context-enhanced.
//
// Enhancement is best-effort by design: a failing enhancer is logged and
// skipped, and the transition happens regardless — enrichment must never
// block the pipeline.
package contextctrl
