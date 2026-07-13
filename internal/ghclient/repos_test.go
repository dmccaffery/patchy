// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package ghclient

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

func TestDefaultBranch(t *testing.T) {
	mux, c := newFakeClient(t)
	mux.HandleFunc("GET /repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		wantHeader(t, r, "Authorization", "Bearer pat-token")
		writeJSON(t, w, `{"name":"r","default_branch":"trunk"}`)
	})

	got, err := c.DefaultBranch(context.Background(), testRepo)
	if err != nil {
		t.Fatalf("DefaultBranch() error = %v", err)
	}
	if got != "trunk" {
		t.Errorf("DefaultBranch() = %q, want %q", got, "trunk")
	}
}

func TestCreatePR(t *testing.T) {
	mux, c := newFakeClient(t)
	mux.HandleFunc("POST /repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		want := map[string]any{
			"title": "fix: patch it",
			"head":  "patchy/fix-4",
			"base":  "main",
			"body":  "closes #3",
		}
		if body := decodeBody[map[string]any](t, r); !reflect.DeepEqual(body, want) {
			t.Errorf("create PR request = %v, want %v", body, want)
		}
		writeJSON(t, w, `{"number":12,"html_url":"https://gh/o/r/pull/12"}`)
	})

	got, err := c.CreatePR(context.Background(), testRepo, PRRequest{
		Title: "fix: patch it", Head: "patchy/fix-4", Base: "main", Body: "closes #3",
	})
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}
	want := &PR{Number: 12, HTMLURL: "https://gh/o/r/pull/12"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CreatePR() = %+v, want %+v", got, want)
	}
}

func TestSearchIssues(t *testing.T) {
	mux, c := newFakeClient(t)
	const query = `label:"security-finding: opened" is:open`
	page1 := `{"total_count":2,"items":[
		{"number":1,"title":"t1","state":"open",
		 "repository_url":"https://api.github.com/repos/o/r"}]}`
	page2 := `{"total_count":2,"items":[
		{"number":2,"title":"t2","state":"open",
		 "repository_url":"https://api.github.com/repos/o2/r2"}]}`
	paged := pagedHandler(t, page1, page2)
	mux.HandleFunc("GET /search/issues", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != query {
			t.Errorf("q = %q, want %q", got, query)
		}
		paged(w, r)
	})

	got, err := c.SearchIssues(context.Background(), query)
	if err != nil {
		t.Fatalf("SearchIssues() error = %v", err)
	}
	want := []*Issue{
		{Repo: Repo{Owner: "o", Name: "r"}, Number: 1, Title: "t1", State: "open"},
		{Repo: Repo{Owner: "o2", Name: "r2"}, Number: 2, Title: "t2", State: "open"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SearchIssues() = %+v, want %+v", got, want)
	}
}
