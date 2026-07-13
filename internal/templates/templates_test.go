// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package templates

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bitwise-media-group/patchy/pkg/source"
)

var update = flag.Bool("update", false, "rewrite golden files")

func testManifest() Manifest {
	return Manifest{
		Source:      "ghas",
		Advisories:  []string{"CWE-79", "CVE-2026-1234"},
		RuleID:      "js/reflected-xss",
		Title:       "Reflected cross-site scripting",
		Description: "Directly writing user input to the page allows XSS.\n\nSanitize all user input.",
		Severity:    "high",
		Alerts: []Alert{
			{
				Number:  7,
				HTMLURL: "https://github.com/acme/shop/security/code-scanning/7",
				Locations: []source.Location{
					{Path: "src/render.js", StartLine: 42, EndLine: 44},
				},
			},
		},
	}
}

// golden compares got with the named golden file, rewriting it under -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("update golden %s: %v", path, err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestGoldens(t *testing.T) {
	m := testManifest()
	tests := []struct {
		name   string
		render func() (string, error)
	}{
		{"issue.md", func() (string, error) { return RenderIssueBody(m) }},
		{"comment_accumulation.md", func() (string, error) { return AccumulationComment(m.Alerts[0]) }},
		{"comment_enhancement.md", func() (string, error) {
			return EnhancementComment("cmdb", "**Owners:** @octocat\n**System:** storefront")
		}},
		{"comment_report.md", func() (string, error) { return ReportComment("Classification", "report body here") }},
		{"comment_approve.md", func() (string, error) {
			return ApproveComment("Remediation confidence 0.61 is below the 0.75 threshold.")
		}},
		{"pr_body.md", func() (string, error) { return PRBody(123, "remediation report here") }},
		{"workspace_issue.md", func() (string, error) {
			return RenderWorkspaceIssue(WorkspaceIssue{
				Repo: "acme/shop", Number: 123, Title: "[ghas] CWE-79: Reflected cross-site scripting",
				Body:     "issue body with manifest",
				Comments: []string{"### Context — `cmdb`\n\n**Owners:** @octocat", "accumulated alert #9"},
			})
		}},
		{"prompt_classify.md", func() (string, error) {
			return RenderClassifyPrompt(ClassifyPrompt{
				IssuePath:          "/workspace/input/issue.md",
				ReportPath:         "/workspace/reports/classification.md",
				AllowedModels:      []string{"claude-sonnet-5", "claude-opus-4-8"},
				MaxTurnsCeiling:    80,
				TokenBudgetCeiling: 400000,
			})
		}},
		{"prompt_remediate.md", func() (string, error) {
			return RenderRemediatePrompt(RemediatePrompt{
				IssuePath:          "/workspace/input/issue.md",
				ClassificationPath: "/workspace/reports/classification.md",
				ReportPath:         "/workspace/reports/remediation.md",
				CommitScriptPath:   "/workspace/commit.sh",
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.render()
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			golden(t, tt.name, got)
		})
	}
}

func TestManifestRoundTrip(t *testing.T) {
	want := testManifest()
	body, err := RenderIssueBody(want)
	if err != nil {
		t.Fatalf("RenderIssueBody: %v", err)
	}
	got, err := ParseManifest(body)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
}

func TestManifestAdd(t *testing.T) {
	m := testManifest()
	dup := source.Finding{AlertNumber: 7}
	if m.Add(dup) {
		t.Error("Add(existing alert) = true, want false")
	}
	fresh := source.Finding{
		AlertNumber: 9,
		HTMLURL:     "https://github.com/acme/shop/security/code-scanning/9",
		Locations:   []source.Location{{Path: "src/other.js", StartLine: 3}},
	}
	if !m.Add(fresh) {
		t.Fatal("Add(new alert) = false, want true")
	}
	if want := []int{7, 9}; !reflect.DeepEqual(m.AlertNumbers(), want) {
		t.Errorf("AlertNumbers() = %v, want %v", m.AlertNumbers(), want)
	}
}

func TestParseManifestErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"no block", "just a hand-written issue"},
		{"unterminated", manifestOpen + `{"source":"ghas"}`},
		{"bad json", manifestOpen + "{nope}" + manifestClose},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseManifest(tt.body); err == nil {
				t.Error("ParseManifest() error = nil, want error")
			}
		})
	}
}

func TestIssueTitle(t *testing.T) {
	if got, want := IssueTitle(testManifest()), "[ghas] CWE-79: Reflected cross-site scripting"; got != want {
		t.Errorf("IssueTitle() = %q, want %q", got, want)
	}
}

// TestManifestSurvivesHostileDescription guards the HTML-comment embedding:
// a description containing the comment terminator must not corrupt the block.
func TestManifestSurvivesHostileDescription(t *testing.T) {
	want := testManifest()
	want.Description = "injected --> <!-- patchy:manifest v1 {} -->"
	body, err := RenderIssueBody(want)
	if err != nil {
		t.Fatalf("RenderIssueBody: %v", err)
	}
	got, err := ParseManifest(body)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if got.Description != want.Description {
		t.Errorf("Description = %q, want %q", got.Description, want.Description)
	}
}
