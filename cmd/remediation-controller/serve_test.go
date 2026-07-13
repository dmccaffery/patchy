// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/bitwise-media-group/patchy/internal/cli"
	"github.com/bitwise-media-group/patchy/internal/jobs"
)

// TestAgentEnvReachesThePod guards the seam DESIGN cares about: the harness
// and budget configuration an operator sets on the controller must arrive in
// the agent pod, where agentrun.FromEnv reads it. Nothing else carries it.
func TestAgentEnvReachesThePod(t *testing.T) {
	t.Setenv("PATCHY_CLASSIFY_HARNESS", "fake")
	t.Setenv("PATCHY_REMEDIATE_HARNESS", "fake")

	opts := cli.NewOptions()
	root := cli.NewControllerRoot("remediation-controller", "test", opts)
	cmd := newServeCmd(opts)
	root.AddCommand(cmd)
	root.SetArgs([]string{"serve", "--classify-model", "claude-opus-4-8", "--remediate-token-budget", "12345"})
	// PersistentPreRunE resolves flags and env; the command body needs a
	// cluster, so stop before it runs.
	cmd.RunE = func(*cobra.Command, []string) error { return nil }
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	env := agentEnv(opts)
	for key, want := range map[string]string{
		"PATCHY_CLASSIFY_HARNESS":       "fake",            // env
		"PATCHY_REMEDIATE_HARNESS":      "fake",            // env
		"PATCHY_CLASSIFY_MODEL":         "claude-opus-4-8", // flag
		"PATCHY_REMEDIATE_TOKEN_BUDGET": "12345",           // flag
		"PATCHY_CONFIDENCE_THRESHOLD":   "0.75",            // default
		"PATCHY_CLASSIFY_TIMEOUT":       "15m0s",           // default
	} {
		if got := env[key]; got != want {
			t.Errorf("agentEnv()[%q] = %q, want %q", key, got, want)
		}
	}

	// The jobs package must pass these through rather than drop them.
	for key := range env {
		if reserved(key) {
			t.Errorf("%q collides with a name the jobs package owns; it would never reach the pod", key)
		}
	}
}

// reserved reports whether the jobs package sets this env var itself (in
// which case a Config.Env entry of the same name is ignored).
func reserved(key string) bool {
	switch key {
	case "HOME", "ANTHROPIC_API_KEY", "GITHUB_TOKEN",
		"PATCHY_WORKSPACE", "PATCHY_REPO", "PATCHY_ISSUE", "PATCHY_PHASE", "PATCHY_DEFAULT_BRANCH":
		return true
	}
	return false
}

// TestJobConfigAcceptsAgentEnv proves the env survives into a real Job spec.
func TestJobConfigAcceptsAgentEnv(t *testing.T) {
	cfg := jobs.Config{
		Namespace: "patchy-agents",
		Image:     "example/agent:v1",
		Env:       map[string]string{"PATCHY_CLASSIFY_HARNESS": "fake"},
	}
	if got := cfg.Env["PATCHY_CLASSIFY_HARNESS"]; got != "fake" {
		t.Fatalf("jobs.Config.Env = %v", cfg.Env)
	}
}
