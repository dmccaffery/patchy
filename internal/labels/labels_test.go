// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package labels

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func fullSet() Set {
	return Set{
		Source:         "ghas",
		Advisories:     []string{"CWE-79", "GHSA-xxxx-yyyy-zzzz"},
		Finding:        State("in-review"),
		Severity:       LevelHigh,
		Priority:       LevelMedium,
		Recommendation: RecommendationRemediate,
		Context:        map[string]string{"environment": "prod", "system": "storefront"},
	}
}

func TestRenderParseRoundTrip(t *testing.T) {
	want := fullSet()
	got := Parse(want.Render())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(Render(set)) = %+v, want %+v", got, want)
	}
}

func TestRenderDeterministicAndCapped(t *testing.T) {
	s := fullSet()
	s.Advisories = append(s.Advisories, "GHSA-aaaa-bbbb-cccc-very-long-advisory-identifier")
	first := s.Render()
	second := s.Render()
	if !slices.Equal(first, second) {
		t.Error("Render() is not deterministic")
	}
	for _, name := range first {
		if len(name) > MaxLen {
			t.Errorf("label %q exceeds MaxLen (%d chars)", name, len(name))
		}
	}
}

func TestRenderSkipsZeroFields(t *testing.T) {
	got := (Set{Finding: State("opened")}).Render()
	want := []string{"security-finding: opened"}
	if !slices.Equal(got, want) {
		t.Errorf("Render() = %v, want %v", got, want)
	}
}

func TestContextLabels(t *testing.T) {
	got := (Set{Context: map[string]string{"tier": "1", "environment": "prod"}}).Render()
	want := []string{"security-context: environment=prod", "security-context: tier=1"}
	if !slices.Equal(got, want) {
		t.Errorf("Render() = %v, want %v", got, want)
	}
	parsed := Parse(append(got, "security-context: malformed"))
	if !reflect.DeepEqual(parsed.Context, map[string]string{"tier": "1", "environment": "prod"}) {
		t.Errorf("Parse().Context = %v", parsed.Context)
	}
}

func TestContextLabelsCapped(t *testing.T) {
	longKey := strings.Repeat("k", MaxLen)
	got := (Set{Context: map[string]string{
		"project": "a-very-long-project-identifier-that-cannot-possibly-fit",
		longKey:   "dropped",
	}}).Render()
	// The oversized value truncates to MaxLen; the oversized key is excluded
	// outright rather than corrupted.
	want := []string{"security-context: project=a-very-long-project-iden"}
	if !slices.Equal(got, want) {
		t.Errorf("Render() = %v, want %v", got, want)
	}
	if len(got[0]) != MaxLen {
		t.Errorf("len = %d, want exactly MaxLen", len(got[0]))
	}
}

func TestParseTolerant(t *testing.T) {
	got := Parse([]string{
		"bug",                          // human label, no colon
		"security-finding: opened",     // ours
		"security-finding-bogus: what", // unknown key
		"security-alert: 42",           // retired key
		"security-confidence",          // no value
		"help wanted",
	})
	want := Set{Finding: State("opened")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %+v, want %+v", got, want)
	}
}

func TestParseAcceptsTightColon(t *testing.T) {
	got := Parse([]string{"security-finding:investigating", "security-advisory:CVE-2026-1234"})
	if got.Finding != State("investigating") {
		t.Errorf("Finding = %q, want investigating", got.Finding)
	}
	if want := []string{"CVE-2026-1234"}; !slices.Equal(got.Advisories, want) {
		t.Errorf("Advisories = %v, want %v", got.Advisories, want)
	}
}

func TestDiff(t *testing.T) {
	prev := Set{Finding: State("enhanced"), Source: "ghas"}
	next := prev
	next.Finding = State("investigating")

	add, remove := Diff(prev, next)
	if want := []string{"security-finding: investigating"}; !slices.Equal(add, want) {
		t.Errorf("add = %v, want %v", add, want)
	}
	if want := []string{"security-finding: enhanced"}; !slices.Equal(remove, want) {
		t.Errorf("remove = %v, want %v", remove, want)
	}
}
