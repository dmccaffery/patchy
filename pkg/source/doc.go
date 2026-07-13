// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package source is patchy's public plugin seam for security-finding
// sources. A source turns tool-specific webhook deliveries (GitHub Advanced
// Security code-scanning alerts, other SAST tools, agentic findings) into
// normalized Findings; the source-controller owns everything downstream —
// issue creation, accumulation, and labels — so a new source implements
// Handler and registers it, nothing more.
//
// Implementations may live outside this repository; the exported API is
// deliberately self-contained and depends on nothing under internal/.
// Dependencies a handler needs to enrich a delivery (an API client to fetch
// the full alert, say) are injected at construction, not passed per call.
package source
