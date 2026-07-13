// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package labels

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func fullSet() Set {
	return Set{
		Source:         "ghas",
		Advisories:     []string{"CWE-79", "GHSA-xxxx-yyyy-zzzz"},
		Alerts:         []int{7, 42},
		Finding:        StateClassified,
		Accumulation:   AccumulationComplete,
		Severity:       LevelHigh,
		Priority:       LevelMedium,
		Recommendation: RecommendationRemediate,
		Confidence:     new(0.82),
		TokenBudget:    200000,
		MaxTurns:       40,
		Attempts:       1,
		Classifier:     "claude",
		Classification: &Usage{
			InputTokens:  1234,
			OutputTokens: 5678,
			Turns:        9,
			CostUSD:      0.42,
			Session:      "a1b2c3d4", // already label-length
			Elapsed:      93100 * time.Millisecond,
		},
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
	s.Classification.Session = "0123456789abcdef0123456789abcdef" // long uuid-ish
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
	if want := "security-classification-session: 01234567"; !slices.Contains(first, want) {
		t.Errorf("session label not truncated to %q in %v", want, first)
	}
}

func TestRenderSkipsZeroFields(t *testing.T) {
	got := (Set{Finding: StateOpened, Accumulation: AccumulationOpen}).Render()
	want := []string{"security-finding: opened", "security-accumulation: open"}
	if !slices.Equal(got, want) {
		t.Errorf("Render() = %v, want %v", got, want)
	}
}

func TestParseTolerant(t *testing.T) {
	got := Parse([]string{
		"bug",                          // human label, no colon
		"security-finding: opened",     // ours
		"security-finding-bogus: what", // unknown key
		"security-alert: not-a-number", // unparseable value
		"security-confidence",          // no value
		"help wanted",
	})
	want := Set{Finding: StateOpened}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %+v, want %+v", got, want)
	}
}

func TestParseAcceptsTightColon(t *testing.T) {
	got := Parse([]string{"security-finding:classifying", "security-advisory:CVE-2026-1234"})
	if got.Finding != StateClassifying {
		t.Errorf("Finding = %q, want classifying", got.Finding)
	}
	if want := []string{"CVE-2026-1234"}; !slices.Equal(got.Advisories, want) {
		t.Errorf("Advisories = %v, want %v", got.Advisories, want)
	}
}

func TestDiff(t *testing.T) {
	prev := Set{Finding: StateContextEnhanced, Accumulation: AccumulationComplete, Source: "ghas"}
	next := prev
	next.Finding = StateClassifying

	add, remove := Diff(prev, next)
	if want := []string{"security-finding: classifying"}; !slices.Equal(add, want) {
		t.Errorf("add = %v, want %v", add, want)
	}
	if want := []string{"security-finding: context-enhanced"}; !slices.Equal(remove, want) {
		t.Errorf("remove = %v, want %v", remove, want)
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0.75, "0.75"},
		{0.4123, "0.4123"},
		{0.42, "0.42"},
		{1, "1"},
		{0.75001, "0.75"}, // capped at 4 decimals, trimmed
	}
	for _, tt := range tests {
		if got := formatFloat(tt.in); got != tt.want {
			t.Errorf("formatFloat(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUsageElapsedRoundTrip(t *testing.T) {
	s := Set{Classification: &Usage{Elapsed: 93100 * time.Millisecond}}
	names := s.Render()
	i := slices.IndexFunc(names, func(n string) bool {
		return strings.HasPrefix(n, "security-classification-elapsed")
	})
	if i < 0 {
		t.Fatalf("no elapsed label in %v", names)
	}
	if want := "security-classification-elapsed: 93.1s"; names[i] != want {
		t.Errorf("elapsed label = %q, want %q", names[i], want)
	}
	if got := Parse(names).Classification.Elapsed; got != 93100*time.Millisecond {
		t.Errorf("round-tripped Elapsed = %v, want 93.1s", got)
	}
}
