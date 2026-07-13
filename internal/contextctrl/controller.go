// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package contextctrl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/labels"
	"github.com/bitwise-media-group/patchy/internal/templates"
	"github.com/bitwise-media-group/patchy/internal/webhook"
	"github.com/bitwise-media-group/patchy/pkg/enhance"
	"github.com/bitwise-media-group/patchy/pkg/source"
)

// Stores is the per-repository GitHub surface the controller mutates.
type Stores interface {
	ghclient.IssueStore
}

// Searcher runs cross-repository issue searches within one installation.
type Searcher interface {
	SearchIssues(ctx context.Context, query string) ([]*ghclient.Issue, error)
}

// Clients resolves GitHub access per repository and across installations.
type Clients interface {
	For(ctx context.Context, repo ghclient.Repo) (Stores, error)
	All(ctx context.Context) ([]Searcher, error)
}

// Config tunes the controller.
type Config struct {
	// Grace is how old an opened issue must be before the reconcile pass
	// enhances it — breathing room for the webhook fast path so the two
	// don't race on fresh issues. Default two minutes.
	Grace time.Duration
}

// Controller is the context-controller engine; it implements
// webhook.Handler.
type Controller struct {
	log       *slog.Logger
	clients   Clients
	enhancers []enhance.Enhancer
	grace     time.Duration
	now       func() time.Time
}

// New builds a Controller running the given enhancer chain, in order.
func New(log *slog.Logger, clients Clients, cfg Config, chain ...enhance.Enhancer) *Controller {
	if cfg.Grace <= 0 {
		cfg.Grace = 2 * time.Minute
	}
	return &Controller{
		log:       log,
		clients:   clients,
		enhancers: chain,
		grace:     cfg.Grace,
		now:       time.Now,
	}
}

// issuesEvent is the slice of the issues webhook payload we consume.
type issuesEvent struct {
	Action string `json:"action"`
	Issue  struct {
		Number int `json:"number"`
	} `json:"issue"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// Handle implements webhook.Handler for issues events.
func (c *Controller) Handle(ctx context.Context, e webhook.Event) error {
	if e.Type != "issues" {
		return nil
	}
	var ev issuesEvent
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return fmt.Errorf("decode issues payload: %w", err)
	}
	if ev.Action != "opened" && ev.Action != "labeled" {
		return nil
	}
	repo := ghclient.Repo{Owner: ev.Repository.Owner.Login, Name: ev.Repository.Name}
	if repo.Owner == "" || repo.Name == "" || ev.Issue.Number == 0 {
		return nil
	}
	return c.process(ctx, repo, ev.Issue.Number)
}

// process enhances one issue if (re-reading current state) it still awaits
// enhancement. The fresh read makes redeliveries and webhook/reconcile
// overlap idempotent: an issue is only ever enhanced while it carries
// security-finding: opened.
func (c *Controller) process(ctx context.Context, repo ghclient.Repo, number int) error {
	st, err := c.clients.For(ctx, repo)
	if err != nil {
		return err
	}
	awaiting, err := st.ListOpen(ctx, repo, []string{
		labels.Name(labels.KeyFinding, string(labels.StateOpened)),
	})
	if err != nil {
		return err
	}
	for _, issue := range awaiting {
		if issue.Number == number {
			return c.enhanceIssue(ctx, st, issue)
		}
	}
	return nil // already enhanced, closed, or not a finding issue
}

// enhanceIssue runs the chain and advances the state. Enhancer failures are
// logged and skipped; only GitHub write failures propagate (the reconcile
// pass retries those).
func (c *Controller) enhanceIssue(ctx context.Context, st Stores, issue *ghclient.Issue) error {
	in := enhance.Issue{
		Repo:   source.Repo{Owner: issue.Repo.Owner, Name: issue.Repo.Name},
		Number: issue.Number,
		Title:  issue.Title,
		Body:   issue.Body,
		Labels: issue.Labels,
	}
	for _, e := range c.enhancers {
		enr, err := e.Enhance(ctx, in)
		if err != nil {
			c.log.LogAttrs(ctx, slog.LevelWarn, "enhancer failed",
				slog.String("enhancer", e.ID()),
				slog.String("repo", issue.Repo.String()),
				slog.Int("issue", issue.Number),
				slog.Any("error", err))
			continue
		}
		if enr == nil {
			continue
		}
		comment, err := templates.RenderEnrichmentComment(templates.Enrichment{
			Enhancer:   e.ID(),
			Owners:     enr.Owners,
			Attributes: enr.Attributes,
		}, enr.CommentMarkdown)
		if err != nil {
			return err
		}
		if err := st.Comment(ctx, issue.Repo, issue.Number, comment); err != nil {
			return err
		}
	}

	if err := st.AddLabels(ctx, issue.Repo, issue.Number,
		[]string{labels.Name(labels.KeyFinding, string(labels.StateContextEnhanced))}); err != nil {
		return err
	}
	if err := st.RemoveLabel(ctx, issue.Repo, issue.Number,
		labels.Name(labels.KeyFinding, string(labels.StateOpened))); err != nil {
		return err
	}
	c.log.LogAttrs(ctx, slog.LevelInfo, "context enhanced",
		slog.String("repo", issue.Repo.String()),
		slog.Int("issue", issue.Number))
	return nil
}

// Reconcile sweeps for opened issues the webhook path missed, enhancing any
// older than the grace period.
func (c *Controller) Reconcile(ctx context.Context) error {
	searchers, err := c.clients.All(ctx)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("is:issue is:open label:%q",
		labels.Name(labels.KeyFinding, string(labels.StateOpened)))

	var errs []error
	for _, s := range searchers {
		issues, err := s.SearchIssues(ctx, query)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, issue := range issues {
			if c.now().Sub(issue.CreatedAt) < c.grace {
				continue
			}
			st, err := c.clients.For(ctx, issue.Repo)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := c.enhanceIssue(ctx, st, issue); err != nil {
				errs = append(errs, fmt.Errorf("issue %s#%d: %w", issue.Repo, issue.Number, err))
			}
		}
	}
	return errors.Join(errs...)
}
