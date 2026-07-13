// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package labels

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from State
		to   State
		want bool
	}{
		{"new issue opens", "", StateOpened, true},
		{"opened enhances", StateOpened, StateContextEnhanced, true},
		{"enhanced classifies", StateContextEnhanced, StateClassifying, true},
		{"classifying classifies", StateClassifying, StateClassified, true},
		{"classifying reverts for retry", StateClassifying, StateContextEnhanced, true},
		{"classifying exhausts to attempted", StateClassifying, StateAttempted, true},
		{"classified re-runs via approve", StateClassified, StateClassifying, true},
		{"classified reaches review", StateClassified, StateInReview, true},
		{"classified fails to attempted", StateClassified, StateAttempted, true},
		{"review merges", StateInReview, StateRemediated, true},
		{"review closes unmerged", StateInReview, StateAttempted, true},
		{"self transition is a no-op", StateClassified, StateClassified, true},

		{"opened cannot skip to classifying", StateOpened, StateClassifying, false},
		{"opened cannot jump to review", StateOpened, StateInReview, false},
		{"enhanced cannot classify directly", StateContextEnhanced, StateClassified, false},
		{"remediated is terminal", StateRemediated, StateClassifying, false},
		{"attempted is terminal", StateAttempted, StateClassifying, false},
		{"new issue cannot start enhanced", "", StateContextEnhanced, false},
		{"no backwards to opened", StateContextEnhanced, StateOpened, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTerminal(t *testing.T) {
	for s, want := range map[State]bool{
		StateRemediated:      true,
		StateAttempted:       true,
		StateOpened:          false,
		StateClassifying:     false,
		State("unknown"):     false,
		StateContextEnhanced: false,
	} {
		if got := Terminal(s); got != want {
			t.Errorf("Terminal(%q) = %v, want %v", s, got, want)
		}
	}
}
