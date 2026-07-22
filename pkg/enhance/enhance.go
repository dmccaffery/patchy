// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package enhance

import (
	"context"

	"github.com/bitwise-media-group/patchy/pkg/source"
)

// Issue is the minimal view of a security-finding issue an Enhancer sees.
type Issue struct {
	Repo   source.Repo
	Number int
	Title  string
	Body   string
	Labels []string
}

// Enrichment is what an Enhancer contributes to an issue.
type Enrichment struct {
	// Owners are GitHub logins responsible for the affected repository, in
	// preference order; the pipeline uses them for issue assignment.
	Owners []string
	// CommentMarkdown is arbitrary content, kept as one sticky comment per
	// enhancer on the tracking issue. Empty means no comment. Semi-structured
	// facts belong in Attributes, not here.
	CommentMarkdown string
	// Attributes are semi-structured facts (system name, environment, tier),
	// projected as tracking labels; carried verbatim.
	Attributes map[string]string
}

// Enhancer is the interface a context-enhancement plugin implements.
type Enhancer interface {
	// ID names the enhancer, for logs and comment attribution.
	ID() string
	// Enhance returns the enrichment for the issue, or (nil, nil) when the
	// enhancer has nothing to contribute.
	Enhance(ctx context.Context, issue Issue) (*Enrichment, error)
}
