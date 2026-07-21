// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package kube is the shared controller-runtime scaffolding: the scheme with
// every API group the controllers touch, manager construction honoring the
// --kubeconfig flag, leader election, and the logr→slog bridge. Every
// CRD-watching binary builds its manager here so the knobs stay uniform.
package kube
