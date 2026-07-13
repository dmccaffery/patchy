// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package sourcectrl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/labels"
	"github.com/bitwise-media-group/patchy/internal/templates"
	"github.com/bitwise-media-group/patchy/internal/webhook"
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

// Clients resolves GitHub access: per-repository stores for the webhook
// path, and one searcher per installation for the reconcile sweep.
type Clients interface {
	For(ctx context.Context, repo ghclient.Repo) (Stores, error)
	All(ctx context.Context) ([]Searcher, error)
}

// Config tunes the controller.
type Config struct {
	// Window is the accumulation window; issues older than this stop
	// accumulating and are released downstream. Default one hour.
	Window time.Duration
}

// Controller is the source-controller engine. It implements
// webhook.Handler.
type Controller struct {
	log     *slog.Logger
	clients Clients
	window  time.Duration
	// sources routes webhook event types to the plugins consuming them.
	sources map[string][]source.Handler
	// keys serializes work per accumulation key so concurrent deliveries of
	// the same finding family cannot double-create issues.
	keys keyMutex
	// now is replaceable for tests.
	now func() time.Time
}

// New builds a Controller over the given source plugins.
func New(log *slog.Logger, clients Clients, cfg Config, handlers ...source.Handler) *Controller {
	if cfg.Window <= 0 {
		cfg.Window = time.Hour
	}
	c := &Controller{
		log:     log,
		clients: clients,
		window:  cfg.Window,
		sources: make(map[string][]source.Handler),
		now:     time.Now,
	}
	for _, h := range handlers {
		for _, event := range h.Events() {
			c.sources[event] = append(c.sources[event], h)
		}
	}
	return c
}

// Handle implements webhook.Handler: it routes the delivery to every source
// plugin registered for the event type and processes the findings.
func (c *Controller) Handle(ctx context.Context, e webhook.Event) error {
	var errs []error
	for _, h := range c.sources[e.Type] {
		findings, err := h.Findings(ctx, e.Type, e.Payload)
		if err != nil {
			errs = append(errs, fmt.Errorf("source %s: %w", h.ID(), err))
			continue
		}
		for _, f := range findings {
			if err := c.process(ctx, f); err != nil {
				errs = append(errs, fmt.Errorf("source %s alert %d: %w", h.ID(), f.AlertNumber, err))
			}
		}
	}
	return errors.Join(errs...)
}

// process lands one finding: accumulate into the open issue for its key
// when the window is still open, otherwise flip the aged issue and open a
// fresh one.
func (c *Controller) process(ctx context.Context, f source.Finding) error {
	if len(f.Advisories) == 0 {
		return fmt.Errorf("finding for alert %d carries no advisories", f.AlertNumber)
	}
	repo := ghclient.Repo{Owner: f.Repo.Owner, Name: f.Repo.Name}
	primary := f.Advisories[0]

	unlock := c.keys.lock(repo.String() + "|" + f.Source + "|" + primary)
	defer unlock()

	st, err := c.clients.For(ctx, repo)
	if err != nil {
		return err
	}
	open, err := st.ListOpen(ctx, repo, []string{
		labels.Name(labels.KeySource, f.Source),
		labels.Name(labels.KeyAdvisory, primary),
		labels.Name(labels.KeyAccumulation, string(labels.AccumulationOpen)),
	})
	if err != nil {
		return err
	}

	if len(open) > 0 {
		issue := oldest(open)
		if c.now().Sub(issue.CreatedAt) < c.window {
			return c.accumulate(ctx, st, issue, f)
		}
		// Aged but the reconcile pass has not flipped it yet: complete it
		// now and fall through to a fresh issue.
		if err := c.complete(ctx, st, issue.Repo, issue.Number); err != nil {
			return err
		}
	}
	return c.create(ctx, st, repo, f)
}

// accumulate folds the finding's alert into the issue: manifest first (the
// source of truth), then the derived body, labels, and comment.
func (c *Controller) accumulate(ctx context.Context, st Stores, issue *ghclient.Issue, f source.Finding) error {
	m, err := templates.ParseManifest(issue.Body)
	if err != nil {
		return fmt.Errorf("issue %s#%d: %w", issue.Repo, issue.Number, err)
	}
	if !m.Add(f) {
		// Alert already recorded — a redelivery. Idempotent no-op.
		return nil
	}
	body, err := templates.RenderIssueBody(m)
	if err != nil {
		return err
	}
	if err := st.EditBody(ctx, issue.Repo, issue.Number, body); err != nil {
		return err
	}
	if err := st.AddLabels(ctx, issue.Repo, issue.Number,
		[]string{labels.Name(labels.KeyAlert, strconv.Itoa(f.AlertNumber))}); err != nil {
		return err
	}
	comment, err := templates.AccumulationComment(templates.Alert{
		Number: f.AlertNumber, HTMLURL: f.HTMLURL, Locations: f.Locations,
	})
	if err != nil {
		return err
	}
	if err := st.Comment(ctx, issue.Repo, issue.Number, comment); err != nil {
		return err
	}
	c.log.LogAttrs(ctx, slog.LevelInfo, "accumulated alert",
		slog.String("repo", issue.Repo.String()),
		slog.Int("issue", issue.Number),
		slog.Int("alert", f.AlertNumber))
	return nil
}

// create opens a fresh issue for the finding.
func (c *Controller) create(ctx context.Context, st Stores, repo ghclient.Repo, f source.Finding) error {
	m := templates.NewManifest(f)
	body, err := templates.RenderIssueBody(m)
	if err != nil {
		return err
	}
	set := labels.Set{
		Source:       f.Source,
		Advisories:   f.Advisories,
		Alerts:       []int{f.AlertNumber},
		Finding:      labels.StateOpened,
		Accumulation: labels.AccumulationOpen,
	}
	issue, err := st.Create(ctx, repo, ghclient.IssueRequest{
		Title:  templates.IssueTitle(m),
		Body:   body,
		Labels: set.Render(),
	})
	if err != nil {
		return err
	}
	c.log.LogAttrs(ctx, slog.LevelInfo, "opened finding issue",
		slog.String("repo", repo.String()),
		slog.Int("issue", issue.Number),
		slog.String("advisory", m.Primary()),
		slog.Int("alert", f.AlertNumber))
	return nil
}

// complete flips an issue to accumulation-complete, releasing it to the
// remediation pipeline. Add before remove so the issue never lacks an
// accumulation label.
func (c *Controller) complete(ctx context.Context, st Stores, repo ghclient.Repo, number int) error {
	if err := st.AddLabels(ctx, repo, number,
		[]string{labels.Name(labels.KeyAccumulation, string(labels.AccumulationComplete))}); err != nil {
		return err
	}
	if err := st.RemoveLabel(ctx, repo, number,
		labels.Name(labels.KeyAccumulation, string(labels.AccumulationOpen))); err != nil {
		return err
	}
	c.log.LogAttrs(ctx, slog.LevelInfo, "accumulation complete",
		slog.String("repo", repo.String()),
		slog.Int("issue", number))
	return nil
}

func oldest(issues []*ghclient.Issue) *ghclient.Issue {
	return slices.MinFunc(issues, func(a, b *ghclient.Issue) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
}
