// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package contextctrl

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
	"github.com/bitwise-media-group/patchy/pkg/enhance"
)

var (
	testLog  = slog.New(slog.NewTextHandler(io.Discard, nil))
	testRepo = ghclient.Repo{Owner: "acme", Name: "shop"}
	baseTime = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
)

type fakeClients struct{ store *ghfake.Store }

func (f *fakeClients) For(context.Context, ghclient.Repo) (Stores, error) { return f.store, nil }
func (f *fakeClients) All(context.Context) ([]Searcher, error)            { return []Searcher{f.store}, nil }

type fakeEnhancer struct {
	id  string
	enr *enhance.Enrichment
	err error
}

func (f *fakeEnhancer) ID() string { return f.id }
func (f *fakeEnhancer) Enhance(context.Context, enhance.Issue) (*enhance.Enrichment, error) {
	return f.enr, f.err
}

// seedIssue creates an opened finding issue in the store.
func seedIssue(t *testing.T, store *ghfake.Store, at time.Time) *ghclient.Issue {
	t.Helper()
	store.Now = func() time.Time { return at }
	is, err := store.Create(context.Background(), testRepo, ghclient.IssueRequest{
		Title: "[ghas] CWE-79: XSS",
		Body:  "body",
		Labels: []string{
			"security-source: ghas", "security-advisory: CWE-79",
			"security-finding: opened", "security-accumulation: open",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return is
}

func issuesPayload(number int) []byte {
	return fmt.Appendf(nil, `{"action":"labeled","issue":{"number":%d},
		"repository":{"name":"shop","owner":{"login":"acme"}}}`, number)
}

func TestEnhancesOnWebhook(t *testing.T) {
	store := ghfake.New()
	is := seedIssue(t, store, baseTime)
	c := New(testLog, &fakeClients{store}, Config{},
		&fakeEnhancer{id: "cmdb", enr: &enhance.Enrichment{
			Owners:          []string{"octocat"},
			CommentMarkdown: "**Owners:** @octocat",
			Attributes:      map[string]string{"system": "storefront"},
		}})

	err := c.Handle(context.Background(), webhook.Event{Type: "issues", Payload: issuesPayload(is.Number)})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	got := store.Issues[is.Number].Labels
	if !slices.Contains(got, "security-finding: context-enhanced") {
		t.Errorf("labels %v missing context-enhanced", got)
	}
	if slices.Contains(got, "security-finding: opened") {
		t.Errorf("labels %v still carry opened", got)
	}

	comments := store.Comments[is.Number]
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(comments))
	}
	enr, ok := templates.ParseEnrichment(comments[0])
	if !ok {
		t.Fatalf("comment carries no enrichment block:\n%s", comments[0])
	}
	if enr.Enhancer != "cmdb" || !slices.Equal(enr.Owners, []string{"octocat"}) {
		t.Errorf("enrichment = %+v, want cmdb/octocat", enr)
	}
	if !strings.Contains(comments[0], "**Owners:** @octocat") {
		t.Errorf("comment missing human markdown:\n%s", comments[0])
	}
}

func TestRedeliveryIsIdempotent(t *testing.T) {
	store := ghfake.New()
	is := seedIssue(t, store, baseTime)
	c := New(testLog, &fakeClients{store}, Config{},
		&fakeEnhancer{id: "cmdb", enr: &enhance.Enrichment{CommentMarkdown: "hello"}})

	for range 2 {
		if err := c.Handle(context.Background(),
			webhook.Event{Type: "issues", Payload: issuesPayload(is.Number)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(store.Comments[is.Number]); got != 1 {
		t.Errorf("comments after redelivery = %d, want 1 (second delivery must no-op)", got)
	}
}

func TestEnhancerFailureStillTransitions(t *testing.T) {
	store := ghfake.New()
	is := seedIssue(t, store, baseTime)
	c := New(testLog, &fakeClients{store}, Config{},
		&fakeEnhancer{id: "broken", err: errors.New("cmdb down")},
		&fakeEnhancer{id: "working", enr: &enhance.Enrichment{CommentMarkdown: "still here"}})

	if err := c.Handle(context.Background(),
		webhook.Event{Type: "issues", Payload: issuesPayload(is.Number)}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !slices.Contains(store.Issues[is.Number].Labels, "security-finding: context-enhanced") {
		t.Error("issue not transitioned despite one enhancer failing")
	}
	if got := len(store.Comments[is.Number]); got != 1 {
		t.Errorf("comments = %d, want 1 (only the working enhancer)", got)
	}
}

func TestSkipsIrrelevantEvents(t *testing.T) {
	store := ghfake.New()
	is := seedIssue(t, store, baseTime)
	c := New(testLog, &fakeClients{store}, Config{}, &fakeEnhancer{id: "cmdb"})

	tests := []struct {
		name    string
		event   webhook.Event
		wantErr bool
	}{
		{"wrong type", webhook.Event{Type: "push", Payload: issuesPayload(is.Number)}, false},
		{"closed action", webhook.Event{Type: "issues",
			Payload: fmt.Appendf(nil, `{"action":"closed","issue":{"number":%d},
				"repository":{"name":"shop","owner":{"login":"acme"}}}`, is.Number)}, false},
		{"bad json", webhook.Event{Type: "issues", Payload: []byte("{nope")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Handle(context.Background(), tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("Handle() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
	if !slices.Contains(store.Issues[is.Number].Labels, "security-finding: opened") {
		t.Error("issue transitioned by an irrelevant event")
	}
}

func TestReconcileSweepsAgedIssues(t *testing.T) {
	store := ghfake.New()
	is := seedIssue(t, store, baseTime)
	c := New(testLog, &fakeClients{store}, Config{Grace: 2 * time.Minute},
		&fakeEnhancer{id: "cmdb", enr: &enhance.Enrichment{CommentMarkdown: "ctx"}})

	// Too fresh: grace not elapsed.
	c.now = func() time.Time { return baseTime.Add(time.Minute) }
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if !slices.Contains(store.Issues[is.Number].Labels, "security-finding: opened") {
		t.Fatal("issue enhanced before the grace period")
	}

	// Aged: swept.
	c.now = func() time.Time { return baseTime.Add(5 * time.Minute) }
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if !slices.Contains(store.Issues[is.Number].Labels, "security-finding: context-enhanced") {
		t.Error("aged issue not enhanced by reconcile")
	}
}
