// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package schedule

import (
	"slices"
	"testing"
	"time"
)

var t0 = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

func cand(name string, prio int32, queuedAgo time.Duration) Candidate {
	return Candidate{Name: name, Priority: prio, QueuedAt: t0.Add(-queuedAgo)}
}

func TestPick(t *testing.T) {
	aging := AgingPolicy{Interval: 24 * time.Hour, Cap: 25}
	cases := []struct {
		name    string
		pending []Candidate
		slots   int
		want    []string
	}{
		{"empty queue", nil, 3, nil},
		{"no slots", []Candidate{cand("a", 90, 0)}, 0, nil},
		{"highest priority first", []Candidate{
			cand("low", 10, 0), cand("high", 90, 0), cand("mid", 50, 0),
		}, 2, []string{"high", "mid"}},
		{"fifo tie-break", []Candidate{
			cand("newer", 50, time.Hour), cand("older", 50, 3*time.Hour),
		}, 1, []string{"older"}},
		{"name tie-break is deterministic", []Candidate{
			cand("b", 50, time.Hour), cand("a", 50, time.Hour),
		}, 1, []string{"a"}},
		{"slots exceed queue", []Candidate{cand("only", 10, 0)}, 5, []string{"only"}},
		{"aging lifts a long waiter over a fresh flood", []Candidate{
			cand("fresh-critical", 90, 0),
			cand("ancient-low", 70, 30*24*time.Hour), // +25 capped boost => 95
		}, 1, []string{"ancient-low"}},
		{"aging cap prevents inversion above genuinely higher work", []Candidate{
			cand("fresh-max", 100, 0),
			cand("ancient-low", 70, 300*24*time.Hour), // capped at +25 => 95
		}, 1, []string{"fresh-max"}},
		{"expedited outranks max priority and aging", []Candidate{
			cand("fresh-max", 100, 0),
			cand("ancient-high", 90, 300*24*time.Hour),
			{Name: "expedited-low", Priority: 5, QueuedAt: t0, Expedited: true},
		}, 2, []string{"expedited-low", "ancient-high"}},
		{"expedited order among themselves by priority", []Candidate{
			{Name: "exp-low", Priority: 10, QueuedAt: t0, Expedited: true},
			{Name: "exp-high", Priority: 80, QueuedAt: t0, Expedited: true},
		}, 1, []string{"exp-high"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Pick(c.pending, c.slots, t0, aging)
			if !slices.Equal(got, c.want) {
				t.Errorf("Pick() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestPickWithoutAging(t *testing.T) {
	got := Pick([]Candidate{
		cand("ancient-low", 10, 300*24*time.Hour),
		cand("fresh-high", 50, 0),
	}, 1, t0, AgingPolicy{})
	if !slices.Equal(got, []string{"fresh-high"}) {
		t.Errorf("Pick(no aging) = %v, want fresh-high (no boost)", got)
	}
}

func TestPickDoesNotMutateInput(t *testing.T) {
	pending := []Candidate{cand("b", 1, 0), cand("a", 2, 0)}
	Pick(pending, 2, t0, AgingPolicy{})
	if pending[0].Name != "b" {
		t.Error("Pick reordered the caller's slice")
	}
}
