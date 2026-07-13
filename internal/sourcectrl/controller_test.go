// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package sourcectrl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/ghfake"
	"github.com/bitwise-media-group/patchy/internal/templates"
	"github.com/bitwise-media-group/patchy/internal/webhook"
	"github.com/bitwise-media-group/patchy/pkg/source"
)

var (
	testLog  = slog.New(slog.NewTextHandler(io.Discard, nil))
	testRepo = ghclient.Repo{Owner: "acme", Name: "shop"}
	baseTime = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
)

type fakeClients struct{ store *ghfake.Store }

func (f *fakeClients) For(context.Context, ghclient.Repo) (Stores, error) { return f.store, nil }
func (f *fakeClients) All(context.Context) ([]Searcher, error)            { return []Searcher{f.store}, nil }

// fakeSource emits canned findings for code_scanning_alert deliveries.
type fakeSource struct {
	findings []source.Finding
	err      error
}

func (f *fakeSource) ID() string       { return "ghas" }
func (f *fakeSource) Events() []string { return []string{"code_scanning_alert"} }
func (f *fakeSource) Findings(context.Context, string, []byte) ([]source.Finding, error) {
	return f.findings, f.err
}

func finding(alert int) source.Finding {
	return source.Finding{
		Source:      "ghas",
		Repo:        source.Repo{Owner: "acme", Name: "shop"},
		AlertNumber: alert,
		Advisories:  []string{"CWE-79"},
		RuleID:      "js/xss",
		Title:       "XSS",
		Description: "desc",
		Severity:    "high",
		HTMLURL:     fmt.Sprintf("https://gh/acme/shop/security/code-scanning/%d", alert),
		Locations:   []source.Location{{Path: "a.js", StartLine: 1}},
	}
}

// newController wires a controller over the fake store with both the
// store's and the controller's clocks pinned to at.
func newController(store *ghfake.Store, fs *fakeSource, at time.Time) *Controller {
	store.Now = func() time.Time { return at }
	c := New(testLog, &fakeClients{store}, Config{Window: time.Hour}, fs)
	c.now = func() time.Time { return at }
	return c
}

func deliver(t *testing.T, c *Controller) error {
	t.Helper()
	return c.Handle(context.Background(), webhook.Event{Type: "code_scanning_alert", Payload: []byte("{}")})
}

func TestCreatesIssue(t *testing.T) {
	store := ghfake.New()
	c := newController(store, &fakeSource{findings: []source.Finding{finding(7)}}, baseTime)

	if err := deliver(t, c); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(store.Issues) != 1 {
		t.Fatalf("issues created = %d, want 1", len(store.Issues))
	}
	is := store.Issues[101]
	for _, want := range []string{
		"security-source: ghas",
		"security-advisory: CWE-79",
		"security-alert: 7",
		"security-finding: opened",
		"security-accumulation: open",
	} {
		if !slices.Contains(is.Labels, want) {
			t.Errorf("labels %v missing %q", is.Labels, want)
		}
	}
	if want := "[ghas] CWE-79: XSS"; is.Title != want {
		t.Errorf("title = %q, want %q", is.Title, want)
	}
	m, err := templates.ParseManifest(is.Body)
	if err != nil {
		t.Fatalf("issue body has no manifest: %v", err)
	}
	if want := []int{7}; !slices.Equal(m.AlertNumbers(), want) {
		t.Errorf("manifest alerts = %v, want %v", m.AlertNumbers(), want)
	}
}

func TestAccumulatesWithinWindow(t *testing.T) {
	store := ghfake.New()
	c := newController(store, &fakeSource{findings: []source.Finding{finding(7)}}, baseTime)
	if err := deliver(t, c); err != nil {
		t.Fatalf("first delivery: %v", err)
	}

	// Second alert of the same family, 30 minutes later.
	c2 := newController(store, &fakeSource{findings: []source.Finding{finding(9)}}, baseTime.Add(30*time.Minute))
	if err := deliver(t, c2); err != nil {
		t.Fatalf("second delivery: %v", err)
	}

	if len(store.Issues) != 1 {
		t.Fatalf("issues = %d, want 1 (accumulated, not created)", len(store.Issues))
	}
	is := store.Issues[101]
	m, err := templates.ParseManifest(is.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{7, 9}; !slices.Equal(m.AlertNumbers(), want) {
		t.Errorf("manifest alerts = %v, want %v", m.AlertNumbers(), want)
	}
	if !slices.Contains(is.Labels, "security-alert: 9") {
		t.Errorf("labels %v missing accumulated alert label", is.Labels)
	}
	if got := len(store.Comments[101]); got != 1 {
		t.Errorf("accumulation comments = %d, want 1", got)
	}
}

func TestRedeliveryIsIdempotent(t *testing.T) {
	store := ghfake.New()
	c := newController(store, &fakeSource{findings: []source.Finding{finding(7)}}, baseTime)
	if err := deliver(t, c); err != nil {
		t.Fatal(err)
	}
	if err := deliver(t, c); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if len(store.Issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(store.Issues))
	}
	if got := len(store.Comments[101]); got != 0 {
		t.Errorf("comments after redelivery = %d, want 0", got)
	}
}

func TestAgedIssueFlipsAndCreatesFresh(t *testing.T) {
	store := ghfake.New()
	c := newController(store, &fakeSource{findings: []source.Finding{finding(7)}}, baseTime)
	if err := deliver(t, c); err != nil {
		t.Fatal(err)
	}

	// Two hours later a new alert of the same family arrives.
	c2 := newController(store, &fakeSource{findings: []source.Finding{finding(9)}}, baseTime.Add(2*time.Hour))
	if err := deliver(t, c2); err != nil {
		t.Fatal(err)
	}

	if len(store.Issues) != 2 {
		t.Fatalf("issues = %d, want 2 (aged issue flipped, fresh one created)", len(store.Issues))
	}
	old := store.Issues[101]
	if !slices.Contains(old.Labels, "security-accumulation: complete") {
		t.Errorf("aged issue labels %v missing accumulation complete", old.Labels)
	}
	if slices.Contains(old.Labels, "security-accumulation: open") {
		t.Errorf("aged issue still accumulation open: %v", old.Labels)
	}
	fresh := store.Issues[102]
	m, err := templates.ParseManifest(fresh.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{9}; !slices.Equal(m.AlertNumbers(), want) {
		t.Errorf("fresh issue alerts = %v, want %v", m.AlertNumbers(), want)
	}
}

func TestHandleJoinsSourceErrors(t *testing.T) {
	store := ghfake.New()
	c := newController(store, &fakeSource{err: errors.New("boom")}, baseTime)
	err := deliver(t, c)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("Handle() error = %v, want wrapped source error", err)
	}
}

func TestReconcileFlipsAged(t *testing.T) {
	store := ghfake.New()
	c := newController(store, &fakeSource{findings: []source.Finding{finding(7)}}, baseTime)
	if err := deliver(t, c); err != nil {
		t.Fatal(err)
	}

	// Within the window nothing flips.
	c.now = func() time.Time { return baseTime.Add(30 * time.Minute) }
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if !slices.Contains(store.Issues[101].Labels, "security-accumulation: open") {
		t.Fatal("issue flipped before the window elapsed")
	}

	// Past the window it flips.
	c.now = func() time.Time { return baseTime.Add(2 * time.Hour) }
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	got := store.Issues[101].Labels
	if !slices.Contains(got, "security-accumulation: complete") || slices.Contains(got, "security-accumulation: open") {
		t.Errorf("labels after reconcile = %v, want flipped to complete", got)
	}
}

func TestReconcileMergesDuplicates(t *testing.T) {
	store := ghfake.New()
	c := newController(store, &fakeSource{findings: []source.Finding{finding(7)}}, baseTime)
	if err := deliver(t, c); err != nil {
		t.Fatal(err)
	}

	// Manufacture a newer duplicate directly (the race the per-key mutex
	// prevents within one process, e.g. two replicas).
	m := templates.NewManifest(finding(9))
	body, err := templates.RenderIssueBody(m)
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return baseTime.Add(time.Minute) }
	dup, err := store.Create(context.Background(), testRepo, ghclient.IssueRequest{
		Title: "dup", Body: body,
		Labels: []string{
			"security-source: ghas", "security-advisory: CWE-79",
			"security-finding: opened", "security-accumulation: open", "security-alert: 9",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	c.now = func() time.Time { return baseTime.Add(10 * time.Minute) }
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if !slices.Contains(store.Closed, dup.Number) {
		t.Fatalf("duplicate #%d not closed; closed = %v", dup.Number, store.Closed)
	}
	keeper := store.Issues[101]
	km, err := templates.ParseManifest(keeper.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{7, 9}; !slices.Equal(km.AlertNumbers(), want) {
		t.Errorf("keeper alerts after merge = %v, want %v", km.AlertNumbers(), want)
	}
	if !slices.Contains(keeper.Labels, "security-alert: 9") {
		t.Errorf("keeper labels %v missing merged alert", keeper.Labels)
	}
	if !slices.Contains(keeper.Labels, "security-accumulation: open") {
		t.Errorf("keeper flipped too early: %v", keeper.Labels)
	}
	if got := len(store.Comments[dup.Number]); got != 1 {
		t.Errorf("duplicate closing comments = %d, want 1", got)
	}
}
