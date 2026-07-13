// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v89/github"
)

// Repo identifies a GitHub repository.
type Repo struct {
	Owner string
	Name  string
}

// String renders the repository as "owner/name".
func (r Repo) String() string { return r.Owner + "/" + r.Name }

// Client wraps the GitHub REST API with patchy's narrow, fake-able
// surface. Construct one with NewToken or App.Installation.
type Client struct {
	gh *github.Client
	// token is the personal access token a dev-mode client authenticates
	// with; empty for installation clients, whose credentials are minted
	// per request by the App transport.
	token string
}

// NewToken returns a Client authenticated with a personal access token —
// the dev-mode fallback. baseURL "" means api.github.com.
func NewToken(token, baseURL string) (*Client, error) {
	gh, err := newGitHub(newRetryTransport(), baseURL, github.WithAuthToken(token))
	if err != nil {
		return nil, fmt.Errorf("ghclient: token client: %w", err)
	}
	return &Client{gh: gh, token: token}, nil
}

// newGitHub builds a go-github client on transport, pointed at baseURL
// ("" means api.github.com; anything else goes through WithEnterpriseURLs,
// which appends /api/v3/ for non-api hosts — the GHES convention).
func newGitHub(transport http.RoundTripper, baseURL string,
	extra ...github.ClientOptionsFunc) (*github.Client, error) {
	opts := append([]github.ClientOptionsFunc{github.WithTransport(transport)}, extra...)
	if baseURL != "" {
		opts = append(opts, github.WithEnterpriseURLs(baseURL, baseURL))
	}
	return github.NewClient(opts...)
}

// apiRoot is gh's resolved base URL without the trailing slash — the form
// ghinstallation expects ("https://api.github.com", "https://ghes/api/v3").
func apiRoot(gh *github.Client) string {
	return strings.TrimSuffix(gh.BaseURL(), "/")
}

// Resolver yields clients regardless of auth mode: an App resolves per-repo
// installations and fans out across all of them; a PAT Client is its own
// single-tenant resolver. Controllers depend on this seam so both modes wire
// identically.
type Resolver interface {
	// For returns the client covering repo.
	For(ctx context.Context, repo Repo) (*Client, error)
	// All returns one client per installation (for cross-repo sweeps).
	All(ctx context.Context) ([]*Client, error)
	// ScopedToken mints a credential for git operations against one
	// repository. Under App auth it is short-lived and single-repository;
	// in dev-token mode it is the configured PAT, which is as scoped as
	// that mode can be.
	ScopedToken(ctx context.Context, repo Repo, perms TokenPerms) (string, time.Time, error)
}

// For implements Resolver: a token client covers everything its token can see.
func (c *Client) For(context.Context, Repo) (*Client, error) { return c, nil }

// All implements Resolver for the single-tenant token client.
func (c *Client) All(context.Context) ([]*Client, error) { return []*Client{c}, nil }

// ScopedToken implements Resolver for the dev-mode token client: it returns
// the configured PAT. A PAT cannot be narrowed per repository — that is
// precisely why App auth is the production mode.
func (c *Client) ScopedToken(context.Context, Repo, TokenPerms) (string, time.Time, error) {
	if c.token == "" {
		return "", time.Time{}, errors.New("ghclient: this client has no static token to scope")
	}
	return c.token, time.Time{}, nil
}
