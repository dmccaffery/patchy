// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package webhook is the shared GitHub-webhook HTTP server every controller
// embeds: HMAC-SHA256 signature validation on the raw body, immediate 202
// acknowledgement into a bounded worker pool, delivery-ID deduplication, and
// health/readiness endpoints with graceful drain.
//
// Handlers must be idempotent — GitHub redelivers, the dedup window is
// finite, and every controller pairs the webhook fast path with a reconcile
// loop that converges missed or dropped deliveries anyway.
package webhook
