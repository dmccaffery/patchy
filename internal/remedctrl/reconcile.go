// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package remedctrl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/labels"
)

// Reconcile is the primary driver: the pickup gate is time-based, so no
// webhook can announce it. Each pass reaps orphaned leases, then picks up
// every eligible finding.
func (c *Controller) Reconcile(ctx context.Context) error {
	var errs []error
	// An issue whose lease was reaped this pass is left for the next one:
	// re-running it immediately would spend a second attempt before the
	// cluster has even cleaned up the first Job.
	reaped, err := c.reapOrphans(ctx)
	if err != nil {
		errs = append(errs, err)
	}

	searchers, err := c.clients.All(ctx)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	// The accumulation-complete label already encodes the elapsed window;
	// the age check below is the belt-and-braces DESIGN asks for.
	query := fmt.Sprintf("is:issue is:open label:%q label:%q",
		labels.Name(labels.KeyFinding, string(labels.StateContextEnhanced)),
		labels.Name(labels.KeyAccumulation, string(labels.AccumulationComplete)))

	for _, s := range searchers {
		issues, err := s.SearchIssues(ctx, query)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, issue := range issues {
			if c.now().Sub(issue.CreatedAt) < c.cfg.MinAge {
				continue
			}
			if reaped[issueRef{issue.Repo, issue.Number}.String()] {
				continue
			}
			if err := c.pickupIssue(ctx, issue); err != nil {
				errs = append(errs, fmt.Errorf("issue %s#%d: %w", issue.Repo, issue.Number, err))
			}
		}
	}
	return errors.Join(errs...)
}

// pickupIssue resolves the repository client and runs the agent for one
// eligible issue.
func (c *Controller) pickupIssue(ctx context.Context, issue *ghclient.Issue) error {
	st, err := c.clients.For(ctx, issue.Repo)
	if err != nil {
		return err
	}
	return c.pickup(ctx, st, issue, phaseFull)
}

// reapOrphans releases leases whose Job is gone: a controller that died
// mid-run leaves an issue labelled classifying with no live Job, and nothing
// else would ever move it. It returns the issues it touched, keyed by
// reference, so the same pass does not immediately pick them up again.
func (c *Controller) reapOrphans(ctx context.Context) (map[string]bool, error) {
	reaped := make(map[string]bool)
	owned, err := c.runner.List(ctx)
	if err != nil {
		return reaped, err
	}
	live := make(map[string]bool, len(owned))
	for _, job := range owned {
		if !job.Status.Done {
			live[fmt.Sprintf("%s#%d", job.Repo, job.Issue)] = true
		}
	}

	searchers, err := c.clients.All(ctx)
	if err != nil {
		return reaped, err
	}
	query := fmt.Sprintf("is:issue is:open label:%q",
		labels.Name(labels.KeyFinding, string(labels.StateClassifying)))

	var errs []error
	for _, s := range searchers {
		issues, err := s.SearchIssues(ctx, query)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, issue := range issues {
			ref := issueRef{issue.Repo, issue.Number}
			if live[ref.String()] {
				continue // still running
			}
			st, err := c.clients.For(ctx, issue.Repo)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			c.log.LogAttrs(ctx, slog.LevelWarn, "reaping orphaned lease",
				slog.String("issue", ref.String()))
			reaped[ref.String()] = true
			if err := c.failAttempt(ctx, st, ref, "the agent job disappeared before reporting"); err != nil {
				errs = append(errs, fmt.Errorf("issue %s: %w", ref, err))
			}
		}
	}
	return reaped, errors.Join(errs...)
}
