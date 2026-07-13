// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/bitwise-media-group/patchy/internal/cli"
	"github.com/bitwise-media-group/patchy/internal/ghas"
	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/reconcile"
	"github.com/bitwise-media-group/patchy/internal/sourcectrl"
	"github.com/bitwise-media-group/patchy/internal/telemetry"
	"github.com/bitwise-media-group/patchy/internal/version"
	"github.com/bitwise-media-group/patchy/internal/webhook"
)

func newServeCmd(opts *cli.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the webhook receiver and the accumulation reconcile loop",
		RunE:  func(cmd *cobra.Command, _ []string) error { return serve(cmd.Context(), opts) },
	}
	cmd.Flags().Duration("accumulation-window", time.Hour,
		"how long alerts of one finding type accumulate into a single issue")
	return cmd
}

func serve(ctx context.Context, opts *cli.Options) error {
	prov, shutdown, err := telemetry.Init(ctx, telemetry.Config{
		Dir:            os.Getenv("PATCHY_TELEMETRY_DIR"),
		Level:          opts.LogLevel,
		ServiceName:    "source-controller",
		ServiceVersion: version.Version,
	})
	if err != nil {
		prov.Logger.LogAttrs(ctx, slog.LevelWarn, "telemetry disabled", slog.Any("error", err))
	}
	defer func() { _ = shutdown(context.WithoutCancel(ctx)) }()
	log := prov.Logger

	secret, err := opts.WebhookSecret()
	if err != nil {
		return err
	}
	resolver, err := opts.GitHub()
	if err != nil {
		return err
	}

	ctrl := sourcectrl.New(log, clients{resolver},
		sourcectrl.Config{Window: opts.Duration("accumulation-window")},
		ghas.New(alerts{resolver}))
	srv := webhook.NewServer(webhook.Config{Addr: opts.ListenAddr, Secret: secret}, log, ctrl)

	log.LogAttrs(ctx, slog.LevelInfo, "source-controller starting",
		slog.String("addr", opts.ListenAddr),
		slog.Duration("reconcile_interval", opts.ReconcileInterval),
		slog.Duration("accumulation_window", opts.Duration("accumulation-window")))

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return srv.Run(ctx) })
	g.Go(func() error {
		return reconcile.Run(ctx, log, "source-controller", opts.ReconcileInterval, ctrl.Reconcile)
	})
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// clients adapts ghclient.Resolver to the controller's Clients seam.
type clients struct{ r ghclient.Resolver }

func (c clients) For(ctx context.Context, repo ghclient.Repo) (sourcectrl.Stores, error) {
	return c.r.For(ctx, repo)
}

func (c clients) All(ctx context.Context) ([]sourcectrl.Searcher, error) {
	cs, err := c.r.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sourcectrl.Searcher, len(cs))
	for i, cl := range cs {
		out[i] = cl
	}
	return out, nil
}

// alerts adapts the resolver to the GHAS plugin's alert fetcher.
type alerts struct{ r ghclient.Resolver }

func (a alerts) GetAlert(ctx context.Context, repo ghclient.Repo, number int) (*ghclient.Alert, error) {
	c, err := a.r.For(ctx, repo)
	if err != nil {
		return nil, err
	}
	return c.GetAlert(ctx, repo, number)
}
