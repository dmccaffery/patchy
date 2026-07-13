// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package source

import "context"

// Repo identifies a GitHub repository.
type Repo struct {
	Owner string
	Name  string
}

// String returns "owner/name".
func (r Repo) String() string { return r.Owner + "/" + r.Name }

// Location is one place in the repository a finding points at. The json tags
// are part of the public contract: locations are embedded verbatim in issue
// manifests and agent handoff files.
type Location struct {
	// Path is repository-relative.
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	// Snippet is the offending source excerpt, when the tool provides one.
	Snippet string `json:"snippet,omitempty"`
}

// Finding is a source-agnostic security finding. The source-controller keys
// accumulation on (Repo, Source, primary advisory) and stamps the labels and
// issue content from these fields alone.
type Finding struct {
	// Source is the emitting Handler's ID, e.g. "ghas".
	Source string
	Repo   Repo
	// AlertNumber is the source-native unique finding number (the
	// code-scanning alert number for GHAS).
	AlertNumber int
	// Advisories are the categorization identifiers (CWE/CVE/GHSA numbers),
	// most authoritative first: Advisories[0] is the accumulation key
	// (GHSA over CVE over CWE, per the source's judgement).
	Advisories []string
	// RuleID is the tool's rule identifier, e.g. a CodeQL query id.
	RuleID string
	// Title is a one-line human summary.
	Title string
	// Description is the full markdown help/description for the rule.
	Description string
	// Severity is the tool-reported severity, normalized by the handler to
	// low, medium, high, or critical.
	Severity string
	// HTMLURL links back to the finding in the source tool.
	HTMLURL string
	// Locations are the places the finding was raised, when available.
	Locations []Location
}

// Handler is the interface a finding source implements.
type Handler interface {
	// ID names the source; it becomes the security-source label value.
	ID() string
	// Events lists the GitHub webhook event types the handler consumes,
	// e.g. ["code_scanning_alert"]. The source-controller routes matching
	// deliveries to Findings.
	Events() []string
	// Findings normalizes one webhook delivery into zero or more Findings.
	// A delivery the handler chooses to skip (an action it does not act on,
	// say) returns (nil, nil).
	Findings(ctx context.Context, eventType string, payload []byte) ([]Finding, error)
}
