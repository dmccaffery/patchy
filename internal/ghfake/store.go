// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package ghfake

import (
	"context"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
)

// Store is the in-memory fake. Fields are exported for test assertions;
// take Mu when reading them from concurrent tests.
type Store struct {
	Mu sync.Mutex
	// Issues by number.
	Issues map[int]*ghclient.Issue
	// Comments by issue number, in posting order.
	Comments map[int][]string
	// Closed issue numbers, in closing order.
	Closed []int
	// Assigned logins by issue number.
	Assigned map[int][]string
	// Now stamps CreatedAt on created issues.
	Now func() time.Time

	next int
}

// New builds an empty Store; issue numbers start at 101.
func New() *Store {
	return &Store{
		Issues:   make(map[int]*ghclient.Issue),
		Comments: make(map[int][]string),
		Assigned: make(map[int][]string),
		Now:      time.Now,
		next:     100,
	}
}

// ListOpen implements ghclient.IssueStore.
func (s *Store) ListOpen(_ context.Context, repo ghclient.Repo, labels []string) ([]*ghclient.Issue, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	var out []*ghclient.Issue
	for _, is := range s.Issues {
		if is.State != "open" || is.Repo != repo {
			continue
		}
		if containsAll(is.Labels, labels) {
			out = append(out, is)
		}
	}
	return out, nil
}

// Create implements ghclient.IssueStore.
func (s *Store) Create(_ context.Context, repo ghclient.Repo, req ghclient.IssueRequest) (*ghclient.Issue, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.next++
	is := &ghclient.Issue{
		Repo: repo, Number: s.next, Title: req.Title, Body: req.Body,
		State: "open", Labels: slices.Clone(req.Labels), CreatedAt: s.Now(),
	}
	s.Issues[s.next] = is
	return is, nil
}

// Comment implements ghclient.IssueStore.
func (s *Store) Comment(_ context.Context, _ ghclient.Repo, number int, body string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Comments[number] = append(s.Comments[number], body)
	return nil
}

// ListComments implements ghclient.IssueStore.
func (s *Store) ListComments(_ context.Context, _ ghclient.Repo, number int) ([]*ghclient.Comment, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	out := make([]*ghclient.Comment, len(s.Comments[number]))
	for i, body := range s.Comments[number] {
		out[i] = &ghclient.Comment{ID: int64(i + 1), Body: body}
	}
	return out, nil
}

// EditBody implements ghclient.IssueStore.
func (s *Store) EditBody(_ context.Context, _ ghclient.Repo, number int, body string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Issues[number].Body = body
	return nil
}

// AddLabels implements ghclient.IssueStore.
func (s *Store) AddLabels(_ context.Context, _ ghclient.Repo, number int, add []string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	is := s.Issues[number]
	for _, l := range add {
		if !slices.Contains(is.Labels, l) {
			is.Labels = append(is.Labels, l)
		}
	}
	return nil
}

// RemoveLabel implements ghclient.IssueStore; removing an absent label is
// not an error, matching the real client.
func (s *Store) RemoveLabel(_ context.Context, _ ghclient.Repo, number int, name string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	is := s.Issues[number]
	is.Labels = slices.DeleteFunc(is.Labels, func(l string) bool { return l == name })
	return nil
}

// Assign implements ghclient.IssueStore.
func (s *Store) Assign(_ context.Context, _ ghclient.Repo, number int, logins []string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Assigned[number] = append(s.Assigned[number], logins...)
	return nil
}

// Close implements ghclient.IssueStore.
func (s *Store) Close(_ context.Context, _ ghclient.Repo, number int) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Issues[number].State = "closed"
	s.Closed = append(s.Closed, number)
	return nil
}

var labelQualifier = regexp.MustCompile(`label:"([^"]+)"`)

// SearchIssues supports the controllers' reconcile queries: every
// label:"..." qualifier must match; is:open is honored; everything else in
// the query is ignored.
func (s *Store) SearchIssues(_ context.Context, query string) ([]*ghclient.Issue, error) {
	matches := labelQualifier.FindAllStringSubmatch(query, -1)
	want := make([]string, 0, len(matches))
	for _, m := range matches {
		want = append(want, m[1])
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	var out []*ghclient.Issue
	for _, is := range s.Issues {
		if is.State != "open" {
			continue
		}
		if containsAll(is.Labels, want) {
			out = append(out, is)
		}
	}
	return out, nil
}

func containsAll(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}
