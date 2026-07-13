// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package templates

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Enrichment block markers; like the issue manifest, JSON HTML-escaping
// keeps the payload from terminating the comment.
const (
	enrichmentOpen  = "<!-- patchy:enrichment v1 "
	enrichmentClose = " -->"
)

// Enrichment is the machine-readable part of an enhancement comment: the
// facts downstream automation consumes (owners for assignment, attributes
// for future mapping). The human-readable markdown travels alongside it in
// the same comment.
type Enrichment struct {
	Enhancer   string            `json:"enhancer"`
	Owners     []string          `json:"owners,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// RenderEnrichmentComment renders an enhancement comment: attributed
// human-readable markdown plus the embedded machine block.
func RenderEnrichmentComment(e Enrichment, markdown string) (string, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("encode enrichment: %w", err)
	}
	body, err := EnhancementComment(e.Enhancer, markdown)
	if err != nil {
		return "", err
	}
	return enrichmentOpen + string(raw) + enrichmentClose + "\n" + body, nil
}

// ParseEnrichment recovers the machine block from one comment; ok reports
// whether the comment carries one.
func ParseEnrichment(comment string) (Enrichment, bool) {
	_, rest, found := strings.Cut(comment, enrichmentOpen)
	if !found {
		return Enrichment{}, false
	}
	raw, _, found := strings.Cut(rest, enrichmentClose)
	if !found {
		return Enrichment{}, false
	}
	var e Enrichment
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		return Enrichment{}, false
	}
	return e, true
}
