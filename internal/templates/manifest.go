// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package templates

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/bitwise-media-group/patchy/pkg/source"
)

// Manifest markers. json.Marshal HTML-escapes ">" to >, so the payload
// can never terminate the HTML comment early.
const (
	manifestOpen  = "<!-- patchy:manifest v1 "
	manifestClose = " -->"
)

// Manifest is the machine-readable state embedded in the issue body: the
// finding family the issue tracks and every alert accumulated into it. The
// body is derived from the manifest, so mutating the manifest and
// re-rendering is the only way the body changes.
type Manifest struct {
	Source string `json:"source"`
	// Advisories are the categorization IDs, most authoritative first;
	// Advisories[0] is the accumulation key.
	Advisories []string `json:"advisories"`
	RuleID     string   `json:"rule_id,omitempty"`
	Title      string   `json:"title"`
	// Description is the rule's markdown help text, shared by every alert of
	// the family.
	Description string  `json:"description,omitempty"`
	Severity    string  `json:"severity,omitempty"`
	Alerts      []Alert `json:"alerts"`
}

// Alert is one accumulated finding instance.
type Alert struct {
	Number    int               `json:"number"`
	HTMLURL   string            `json:"url,omitempty"`
	Locations []source.Location `json:"locations,omitempty"`
}

// NewManifest starts a manifest from the finding that opens an issue.
func NewManifest(f source.Finding) Manifest {
	return Manifest{
		Source:      f.Source,
		Advisories:  slices.Clone(f.Advisories),
		RuleID:      f.RuleID,
		Title:       f.Title,
		Description: f.Description,
		Severity:    f.Severity,
		Alerts:      []Alert{alertOf(f)},
	}
}

// Add accumulates a finding's alert into the manifest, reporting whether it
// was new (false means the alert number was already recorded — a redelivery).
func (m *Manifest) Add(f source.Finding) bool {
	if slices.ContainsFunc(m.Alerts, func(a Alert) bool { return a.Number == f.AlertNumber }) {
		return false
	}
	m.Alerts = append(m.Alerts, alertOf(f))
	return true
}

// AlertNumbers lists the accumulated alert numbers in insertion order.
func (m Manifest) AlertNumbers() []int {
	out := make([]int, len(m.Alerts))
	for i, a := range m.Alerts {
		out[i] = a.Number
	}
	return out
}

// Primary is the accumulation-key advisory.
func (m Manifest) Primary() string {
	if len(m.Advisories) == 0 {
		return ""
	}
	return m.Advisories[0]
}

func alertOf(f source.Finding) Alert {
	return Alert{Number: f.AlertNumber, HTMLURL: f.HTMLURL, Locations: slices.Clone(f.Locations)}
}

// renderManifest encodes the manifest block for embedding in an issue body.
func renderManifest(m Manifest) (string, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("encode manifest: %w", err)
	}
	return manifestOpen + string(raw) + manifestClose, nil
}

// ParseManifest recovers the manifest from an issue body written by
// RenderIssueBody.
func ParseManifest(body string) (Manifest, error) {
	_, rest, ok := strings.Cut(body, manifestOpen)
	if !ok {
		return Manifest{}, fmt.Errorf("no manifest block in issue body")
	}
	raw, _, ok := strings.Cut(rest, manifestClose)
	if !ok {
		return Manifest{}, fmt.Errorf("unterminated manifest block in issue body")
	}
	var m Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return m, nil
}
