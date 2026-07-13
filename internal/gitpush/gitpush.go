// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package gitpush

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
)

// Pusher pushes agent changesets.
type Pusher struct{}

// New builds a Pusher.
func New() *Pusher { return &Pusher{} }

// Push unpacks the bundle into a scratch repository and pushes branch to
// the remote. The token is passed to git through an http.extraHeader on the
// command line's -c flags, never written to disk and never persisted in the
// scratch repository's config (it is discarded with the directory).
func (p *Pusher) Push(ctx context.Context, repo ghclient.Repo, cloneURL, token, branch string, bundle []byte) error {
	dir, err := os.MkdirTemp("", "patchy-push-")
	if err != nil {
		return fmt.Errorf("scratch dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	bundlePath := filepath.Join(dir, "changeset.bundle")
	if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}
	work := filepath.Join(dir, "repo")

	// A bundle of base..branch is not self-contained, so the scratch clone
	// must carry the base history the bundle's commits sit on: clone the
	// remote first (shallow would not accept the bundle's parents), then
	// fetch the branch out of the bundle.
	steps := [][]string{
		{"clone", "--no-checkout", cloneURL, work},
		{"-C", work, "fetch", bundlePath, "+" + branch + ":" + branch},
		{"-C", work, "push", "origin", branch + ":" + branch},
	}
	for _, args := range steps {
		if err := run(ctx, dir, token, args...); err != nil {
			return err
		}
	}
	return nil
}

// run executes one git command with the write token supplied as an
// Authorization header for the remote host.
func run(ctx context.Context, dir, token string, args ...string) error {
	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	full := append([]string{
		"-c", "http.extraHeader=AUTHORIZATION: basic " + auth,
		// Never let git prompt or reach a credential helper.
		"-c", "credential.helper=",
	}, args...)

	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Never echo the argv: it carries the Authorization header.
		return fmt.Errorf("git %s: %w: %s", args[0], err, redact(stderr.String(), token))
	}
	return nil
}

// redact removes the token from anything that reaches a log.
func redact(s, token string) string {
	if token == "" {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.ReplaceAll(s, token, "[redacted]"))
}
