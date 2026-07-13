// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package labels

// transitions is the legal state machine. Each component writes only its own
// transitions (source-controller sets the initial opened; context-controller
// owns opened→context-enhanced; remediation-controller owns everything from
// classifying onward), so no label key ever has two writers.
//
//   - classifying→context-enhanced is the retry revert after a failed Job.
//   - classified→classifying is the /approve re-run.
//   - classified→in-review/attempted happen when the remediation stage of the
//     same pod completes after the classification event already advanced the
//     state.
//   - classifying→in-review is the /approve re-run's success: that Job runs
//     the remediation stage only, so no classification event moves the issue
//     out of classifying first.
var transitions = map[State][]State{
	"":                   {StateOpened},
	StateOpened:          {StateContextEnhanced},
	StateContextEnhanced: {StateClassifying},
	StateClassifying:     {StateClassified, StateContextEnhanced, StateInReview, StateAttempted},
	StateClassified:      {StateClassifying, StateInReview, StateAttempted},
	StateInReview:        {StateRemediated, StateAttempted},
	StateRemediated:      nil, // terminal
	StateAttempted:       nil, // terminal
}

// CanTransition reports whether moving the security-finding state from
// `from` to `to` is legal. The empty state means the label is absent (a new
// issue). Self-transitions are always legal no-ops.
func CanTransition(from, to State) bool {
	if from == to {
		return true
	}
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Terminal reports whether the state has no outgoing transitions.
func Terminal(s State) bool {
	next, ok := transitions[s]
	return ok && len(next) == 0
}
