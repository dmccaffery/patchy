// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/remedctrl"
)

// clients adapts ghclient.Resolver to the controller's Clients seam, adding
// the two short-lived token mints. Tokens are minted per operation and never
// cached: the clone token goes to the Job's init container only, the push
// token is used once by the controller itself.
type clients struct{ r ghclient.Resolver }

func (c clients) For(ctx context.Context, repo ghclient.Repo) (remedctrl.Stores, error) {
	return c.r.For(ctx, repo)
}

func (c clients) All(ctx context.Context) ([]remedctrl.Searcher, error) {
	cs, err := c.r.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]remedctrl.Searcher, len(cs))
	for i, cl := range cs {
		out[i] = cl
	}
	return out, nil
}

// CloneToken mints the read-only credential the Job's init container clones
// with — the only GitHub credential that ever reaches the agent pod, and it
// never reaches the agent container itself.
func (c clients) CloneToken(ctx context.Context, repo ghclient.Repo) (string, error) {
	token, _, err := c.r.ScopedToken(ctx, repo, ghclient.TokenPerms{Contents: "read"})
	return token, err
}

// PushToken mints the write credential the controller pushes the agent's
// branch with. It is used once, in-process, and never leaves the controller.
func (c clients) PushToken(ctx context.Context, repo ghclient.Repo) (string, error) {
	token, _, err := c.r.ScopedToken(ctx, repo, ghclient.TokenPerms{Contents: "write"})
	return token, err
}
