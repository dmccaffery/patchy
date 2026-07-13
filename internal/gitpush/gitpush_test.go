// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package gitpush

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
)

// git runs a git command in dir, failing the test on error.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// newOrigin builds a bare "remote" plus a working clone with one commit on
// main, standing in for GitHub — the push path is exercised for real over
// the filesystem transport.
func newOrigin(t *testing.T) (originPath, workPath string) {
	t.Helper()
	root := t.TempDir()
	originPath = filepath.Join(root, "origin.git")
	workPath = filepath.Join(root, "work")

	git(t, root, "init", "--bare", "-b", "main", originPath)
	git(t, root, "clone", originPath, workPath)
	git(t, workPath, "config", "user.email", "test@example.com")
	git(t, workPath, "config", "user.name", "test")
	// The machine's global config may mandate signed commits.
	git(t, workPath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(workPath, "app.js"), []byte("vulnerable();\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, workPath, "add", ".")
	git(t, workPath, "commit", "-m", "initial")
	git(t, workPath, "push", "origin", "main")
	return originPath, workPath
}

// agentBundle simulates the pod: branch off main, commit a fix, and bundle
// main..branch exactly as agent-runner does.
func agentBundle(t *testing.T, workPath, branch string) []byte {
	t.Helper()
	git(t, workPath, "checkout", "-B", branch)
	if err := os.WriteFile(filepath.Join(workPath, "app.js"), []byte("escaped();\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, workPath, "add", "app.js")
	git(t, workPath, "commit", "-m", "fix(security): escape sink")

	bundlePath := filepath.Join(t.TempDir(), "changeset.bundle")
	git(t, workPath, "bundle", "create", bundlePath, "main.."+branch)
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPush(t *testing.T) {
	origin, work := newOrigin(t)
	const branch = "patchy/issue-123"
	bundle := agentBundle(t, work, branch)

	// The filesystem transport ignores the auth header; the token path is
	// exercised, the credential simply is not required by the fake remote.
	err := New().Push(context.Background(), ghclient.Repo{Owner: "acme", Name: "shop"},
		origin, "test-token", branch, bundle)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// The branch and its commit must now exist on the remote.
	refs := git(t, origin, "branch", "--list", branch)
	if !strings.Contains(refs, branch) {
		t.Fatalf("remote branches = %q, want %s", refs, branch)
	}
	subject := git(t, origin, "log", "-1", "--format=%s", branch)
	if subject != "fix(security): escape sink" {
		t.Errorf("pushed commit = %q, want the agent's commit", subject)
	}
	content := git(t, origin, "show", branch+":app.js")
	if content != "escaped();" {
		t.Errorf("pushed content = %q, want the fix", content)
	}
}

func TestPushRejectsCorruptBundle(t *testing.T) {
	origin, _ := newOrigin(t)
	err := New().Push(context.Background(), ghclient.Repo{Owner: "acme", Name: "shop"},
		origin, "test-token", "patchy/issue-1", []byte("not a git bundle"))
	if err == nil {
		t.Fatal("Push() error = nil, want a failure on a corrupt bundle")
	}
}

func TestRedactHidesToken(t *testing.T) {
	got := redact("fatal: could not read Password for 'https://x-access-token:ghs_secret@github.com'", "ghs_secret")
	if strings.Contains(got, "ghs_secret") {
		t.Errorf("redact() leaked the token: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Errorf("redact() = %q, want the token replaced", got)
	}
}
