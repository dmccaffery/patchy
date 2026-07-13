// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package sourcectrl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/labels"
	"github.com/bitwise-media-group/patchy/internal/templates"
)

// Reconcile is the periodic pass: it sweeps every installation for open
// accumulating issues, flips the aged ones, and merges duplicates (two open
// accumulating issues for one key — possible only through races or manual
// label edits) into the oldest. Best-effort: failures are joined, never
// short-circuit the sweep.
func (c *Controller) Reconcile(ctx context.Context) error {
	searchers, err := c.clients.All(ctx)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("is:issue is:open label:%q",
		labels.Name(labels.KeyAccumulation, string(labels.AccumulationOpen)))

	var errs []error
	for _, s := range searchers {
		open, err := s.SearchIssues(ctx, query)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for key, group := range groupByKey(open) {
			if err := c.reconcileGroup(ctx, key, group); err != nil {
				errs = append(errs, fmt.Errorf("key %s: %w", key, err))
			}
		}
	}
	return errors.Join(errs...)
}

// reconcileGroup converges one accumulation key: merge any duplicates into
// the oldest issue, then flip the survivor once aged.
func (c *Controller) reconcileGroup(ctx context.Context, key string, group []*ghclient.Issue) error {
	unlock := c.keys.lock(key)
	defer unlock()

	slices.SortFunc(group, func(a, b *ghclient.Issue) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	keeper := group[0]

	st, err := c.clients.For(ctx, keeper.Repo)
	if err != nil {
		return err
	}
	for _, dup := range group[1:] {
		if err := c.merge(ctx, st, keeper, dup); err != nil {
			return err
		}
	}
	if c.now().Sub(keeper.CreatedAt) >= c.window {
		return c.complete(ctx, st, keeper.Repo, keeper.Number)
	}
	return nil
}

// merge folds a duplicate accumulating issue into the keeper and closes it.
func (c *Controller) merge(ctx context.Context, st Stores, keeper, dup *ghclient.Issue) error {
	dm, err := templates.ParseManifest(dup.Body)
	if err != nil {
		return fmt.Errorf("duplicate %s#%d: %w", dup.Repo, dup.Number, err)
	}
	km, err := templates.ParseManifest(keeper.Body)
	if err != nil {
		return fmt.Errorf("keeper %s#%d: %w", keeper.Repo, keeper.Number, err)
	}

	var added []templates.Alert
	for _, a := range dm.Alerts {
		if !slices.ContainsFunc(km.Alerts, func(k templates.Alert) bool { return k.Number == a.Number }) {
			km.Alerts = append(km.Alerts, a)
			added = append(added, a)
		}
	}
	if len(added) > 0 {
		body, err := templates.RenderIssueBody(km)
		if err != nil {
			return err
		}
		if err := st.EditBody(ctx, keeper.Repo, keeper.Number, body); err != nil {
			return err
		}
		names := make([]string, len(added))
		for i, a := range added {
			names[i] = labels.Name(labels.KeyAlert, fmt.Sprintf("%d", a.Number))
		}
		if err := st.AddLabels(ctx, keeper.Repo, keeper.Number, names); err != nil {
			return err
		}
		keeper.Body = body
	}

	note := fmt.Sprintf("Duplicate of #%d; alerts folded into it. Closing.", keeper.Number)
	if err := st.Comment(ctx, dup.Repo, dup.Number, note); err != nil {
		return err
	}
	if err := st.Close(ctx, dup.Repo, dup.Number); err != nil {
		return err
	}
	c.log.LogAttrs(ctx, slog.LevelWarn, "merged duplicate finding issue",
		slog.String("repo", dup.Repo.String()),
		slog.Int("duplicate", dup.Number),
		slog.Int("keeper", keeper.Number))
	return nil
}

// groupByKey buckets issues by accumulation key (repo|source|primary
// advisory), derived from their labels. Issues missing the labels (foreign
// or hand-mangled) are skipped.
func groupByKey(issues []*ghclient.Issue) map[string][]*ghclient.Issue {
	groups := make(map[string][]*ghclient.Issue)
	for _, is := range issues {
		set := labels.Parse(is.Labels)
		if set.Source == "" || len(set.Advisories) == 0 {
			continue
		}
		// Advisories are sorted by Parse; the primary is not recoverable
		// from labels alone, but any stable pick keys the group
		// consistently, which is all dedup needs.
		key := is.Repo.String() + "|" + set.Source + "|" + set.Advisories[0]
		groups[key] = append(groups[key], is)
	}
	return groups
}
