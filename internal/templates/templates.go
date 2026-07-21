// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package templates

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed *.md.tmpl
var files embed.FS

// tmpl parses every embedded template once; a parse failure is a programmer
// error caught by the package's golden tests, so panicking at init is right.
var tmpl = template.Must(template.New("").
	Funcs(template.FuncMap{"join": strings.Join}).
	ParseFS(files, "*.md.tmpl"))

func render(name string, data any) (string, error) {
	var b strings.Builder
	if err := tmpl.ExecuteTemplate(&b, name, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return b.String(), nil
}

// IssueTitle is the canonical security-finding issue title.
func IssueTitle(m Manifest) string {
	return fmt.Sprintf("[%s] %s: %s", m.Source, m.Primary(), m.Title)
}

// RenderIssueBody renders the full issue body — human summary plus the
// machine manifest block — from the manifest.
func RenderIssueBody(m Manifest) (string, error) {
	block, err := renderManifest(m)
	if err != nil {
		return "", err
	}
	return render("issue.md.tmpl", struct {
		Manifest
		ManifestBlock string
	}{m, block})
}

// AccumulationComment announces an alert being folded into an existing issue.
func AccumulationComment(alert Alert) (string, error) {
	return render("comment_accumulation.md.tmpl", alert)
}

// EnhancementComment wraps an enhancer's contribution with attribution.
func EnhancementComment(enhancerID, markdown string) (string, error) {
	return render("comment_enhancement.md.tmpl", struct {
		ID       string
		Markdown string
	}{enhancerID, markdown})
}

// The stage headings on report comments. They are load-bearing: the
// /approve path recovers the classification report from its comment by
// looking for this heading, so GitHub stays the only state store.
const (
	ClassificationReportHeading = "Classification"
	RemediationReportHeading    = "Remediation"
)

// ReportComment wraps an agent stage report for posting to the issue.
func ReportComment(stage, report string) (string, error) {
	return render("comment_report.md.tmpl", struct {
		Stage  string
		Report string
	}{stage, report})
}

// ApproveComment renders the human instructions for forcing a remediation
// attempt on an issue the pipeline held back.
func ApproveComment(reason string) (string, error) {
	return render("comment_approve.md.tmpl", struct{ Reason string }{reason})
}

// PRBody renders the pull-request body for a remediation branch; issue is
// the tracked issue number ("Fixes #issue" auto-links and auto-closes).
func PRBody(issue int, report string) (string, error) {
	return render("pr_body.md.tmpl", struct {
		Issue  int
		Report string
	}{issue, report})
}

// WorkspaceIssue is the data for the issue handoff file the agent reads.
type WorkspaceIssue struct {
	Repo     string
	Number   int
	Title    string
	Body     string
	Comments []string
}

// RenderWorkspaceIssue renders the workspace issue.md handoff file.
func RenderWorkspaceIssue(w WorkspaceIssue) (string, error) {
	return render("workspace_issue.md.tmpl", w)
}

// ClassifyPrompt is the data for the stage-1 (classification) prompt.
type ClassifyPrompt struct {
	IssuePath          string
	ReportPath         string
	AllowedModels      []string
	MaxTurnsCeiling    int
	TokenBudgetCeiling int
}

// RenderClassifyPrompt renders the classification prompt.
func RenderClassifyPrompt(p ClassifyPrompt) (string, error) {
	return render("prompt_classify.md.tmpl", p)
}

// InvestigatePrompt is the data for the analysis-stage prompt (the split
// pipeline's stage 1: exploitability/likelihood/impact plus the verdict).
type InvestigatePrompt struct {
	IssuePath          string
	ReportPath         string
	AllowedModels      []string
	MaxTurnsCeiling    int
	TokenBudgetCeiling int
}

// RenderInvestigatePrompt renders the investigation prompt.
func RenderInvestigatePrompt(p InvestigatePrompt) (string, error) {
	return render("prompt_investigate.md.tmpl", p)
}

// RemediatePrompt is the data for the stage-2 (remediation) prompt.
type RemediatePrompt struct {
	IssuePath          string
	ClassificationPath string
	ReportPath         string
	CommitScriptPath   string
}

// RenderRemediatePrompt renders the remediation prompt.
func RenderRemediatePrompt(p RemediatePrompt) (string, error) {
	return render("prompt_remediate.md.tmpl", p)
}
