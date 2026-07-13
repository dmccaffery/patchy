// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package enhancers

import (
	"context"

	"github.com/bitwise-media-group/patchy/pkg/enhance"
)

// Noop is the placeholder enhancer used when no context source is
// configured: it records that enhancement ran with nothing to add, so the
// issue still advances through the pipeline.
type Noop struct{}

// ID implements enhance.Enhancer.
func (Noop) ID() string { return "noop" }

// Enhance implements enhance.Enhancer.
func (Noop) Enhance(context.Context, enhance.Issue) (*enhance.Enrichment, error) {
	return &enhance.Enrichment{
		CommentMarkdown: "_No context enhancement sources are configured; proceeding without additional context._",
	}, nil
}
