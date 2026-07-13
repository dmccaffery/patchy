// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package remedctrl

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/labels"
	"github.com/bitwise-media-group/patchy/internal/webhook"
)

// approvers are the author associations allowed to force a remediation
// attempt with /approve.
var approvers = map[string]bool{
	"OWNER":        true,
	"MEMBER":       true,
	"COLLABORATOR": true,
}

// Handle implements webhook.Handler. Pickup is driven by the reconcile loop
// (the gate is time-based, so webhooks alone cannot trigger it); the events
// handled here are the human-in-the-loop ones: /approve comments and pull
// request closures.
func (c *Controller) Handle(ctx context.Context, e webhook.Event) error {
	switch e.Type {
	case "issue_comment":
		return c.handleComment(ctx, e.Payload)
	case "pull_request":
		return c.handlePullRequest(ctx, e.Payload)
	}
	return nil
}

type commentEvent struct {
	Action string `json:"action"`
	Issue  struct {
		Number int    `json:"number"`
		State  string `json:"state"`
	} `json:"issue"`
	Comment struct {
		Body              string `json:"body"`
		AuthorAssociation string `json:"author_association"`
		User              struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
	Repository repository `json:"repository"`
}

type pullRequestEvent struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number int  `json:"number"`
		Merged bool `json:"merged"`
		Head   struct {
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository repository `json:"repository"`
}

type repository struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func (r repository) repo() ghclient.Repo {
	return ghclient.Repo{Owner: r.Owner.Login, Name: r.Name}
}

// handleComment runs the /approve escape hatch: a maintainer forcing a
// remediation attempt the pipeline held back (low confidence, or a
// breaking-change hold).
func (c *Controller) handleComment(ctx context.Context, payload []byte) error {
	var ev commentEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decode issue_comment payload: %w", err)
	}
	if ev.Action != "created" || strings.TrimSpace(ev.Comment.Body) != c.cfg.ApproveCommand {
		return nil
	}
	repo := ev.Repository.repo()
	ref := issueRef{repo, ev.Issue.Number}

	if !approvers[ev.Comment.AuthorAssociation] {
		c.log.LogAttrs(ctx, slog.LevelWarn, "ignoring approve from a non-maintainer",
			slog.String("issue", ref.String()),
			slog.String("user", ev.Comment.User.Login),
			slog.String("association", ev.Comment.AuthorAssociation))
		return nil
	}

	st, err := c.clients.For(ctx, repo)
	if err != nil {
		return err
	}
	issue, err := c.issue(ctx, st, ref)
	if err != nil {
		return err
	}
	set := labels.Parse(issue.Labels)
	if set.Finding != labels.StateClassified {
		c.log.LogAttrs(ctx, slog.LevelInfo, "ignoring approve on an issue that is not classified",
			slog.String("issue", ref.String()), slog.String("state", string(set.Finding)))
		return nil
	}

	c.log.LogAttrs(ctx, slog.LevelInfo, "remediation approved by maintainer",
		slog.String("issue", ref.String()), slog.String("user", ev.Comment.User.Login))
	return c.pickup(ctx, st, issue, phaseRemediate)
}

// handlePullRequest closes the loop: a merged remediation PR marks the
// finding remediated and closes its issue; a PR closed unmerged returns the
// finding to humans.
func (c *Controller) handlePullRequest(ctx context.Context, payload []byte) error {
	var ev pullRequestEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decode pull_request payload: %w", err)
	}
	if ev.Action != "closed" {
		return nil
	}
	number, ok := issueOfBranch(ev.PullRequest.Head.Ref)
	if !ok {
		return nil // not one of ours
	}
	repo := ev.Repository.repo()
	ref := issueRef{repo, number}

	st, err := c.clients.For(ctx, repo)
	if err != nil {
		return err
	}
	if !ev.PullRequest.Merged {
		c.log.LogAttrs(ctx, slog.LevelInfo, "remediation pull request closed unmerged",
			slog.String("issue", ref.String()), slog.Int("pr", ev.PullRequest.Number))
		return c.attempted(ctx, st, ref, fmt.Sprintf(
			"The remediation pull request #%d was closed without merging.", ev.PullRequest.Number))
	}

	if err := c.setFindingLabel(ctx, st, ref, labels.StateRemediated); err != nil {
		return err
	}
	if err := st.Comment(ctx, repo, number, fmt.Sprintf(
		"Remediated by #%d. Closing.", ev.PullRequest.Number)); err != nil {
		return err
	}
	if err := st.Close(ctx, repo, number); err != nil {
		return err
	}
	c.log.LogAttrs(ctx, slog.LevelInfo, "finding remediated",
		slog.String("issue", ref.String()), slog.Int("pr", ev.PullRequest.Number))
	return nil
}

// issueOfBranch recovers the issue number from a patchy remediation branch.
func issueOfBranch(ref string) (int, bool) {
	rest, ok := strings.CutPrefix(ref, "patchy/issue-")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
