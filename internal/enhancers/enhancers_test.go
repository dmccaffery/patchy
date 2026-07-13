// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package enhancers

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bitwise-media-group/patchy/pkg/enhance"
	"github.com/bitwise-media-group/patchy/pkg/source"
)

func TestNoop(t *testing.T) {
	enr, err := Noop{}.Enhance(context.Background(), enhance.Issue{})
	if err != nil {
		t.Fatalf("Enhance() error = %v", err)
	}
	if enr == nil || enr.CommentMarkdown == "" {
		t.Error("noop must contribute an explicit placeholder comment")
	}
	if len(enr.Owners) != 0 {
		t.Errorf("noop owners = %v, want none", enr.Owners)
	}
}

const staticYAML = `
repos:
    acme/shop:
        owners: [octocat, hubot]
        attributes:
            system: storefront
            tier: "1"
    acme/api: {}
`

func writeStatic(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "context.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStaticFile(t *testing.T) {
	s, err := NewStaticFile(writeStatic(t, staticYAML))
	if err != nil {
		t.Fatalf("NewStaticFile() error = %v", err)
	}

	t.Run("known repo", func(t *testing.T) {
		enr, err := s.Enhance(context.Background(), enhance.Issue{
			Repo: source.Repo{Owner: "acme", Name: "shop"},
		})
		if err != nil {
			t.Fatalf("Enhance() error = %v", err)
		}
		if want := []string{"octocat", "hubot"}; !slices.Equal(enr.Owners, want) {
			t.Errorf("Owners = %v, want %v", enr.Owners, want)
		}
		for _, want := range []string{"@octocat", "@hubot", "**system:** storefront", "**tier:** 1"} {
			if !strings.Contains(enr.CommentMarkdown, want) {
				t.Errorf("markdown missing %q:\n%s", want, enr.CommentMarkdown)
			}
		}
		if enr.Attributes["system"] != "storefront" {
			t.Errorf("Attributes = %v", enr.Attributes)
		}
	})

	t.Run("unknown repo contributes nothing", func(t *testing.T) {
		enr, err := s.Enhance(context.Background(), enhance.Issue{
			Repo: source.Repo{Owner: "acme", Name: "other"},
		})
		if err != nil || enr != nil {
			t.Errorf("Enhance() = %v, %v; want nil, nil", enr, err)
		}
	})
}

func TestStaticFileErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(t.TempDir(), "absent.yaml")},
		{"bad yaml", writeStatic(t, "repos: [")},
		{"unknown keys", writeStatic(t, "repositories:\n    a/b: {}\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewStaticFile(tt.path); err == nil {
				t.Error("NewStaticFile() error = nil, want error")
			}
		})
	}
}
