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
	"github.com/bitwise-media-group/patchy/internal/contextctrl"
	"github.com/bitwise-media-group/patchy/internal/enhancers"
	"github.com/bitwise-media-group/patchy/internal/ghclient"
	"github.com/bitwise-media-group/patchy/internal/reconcile"
	"github.com/bitwise-media-group/patchy/internal/telemetry"
	"github.com/bitwise-media-group/patchy/internal/version"
	"github.com/bitwise-media-group/patchy/internal/webhook"
	"github.com/bitwise-media-group/patchy/pkg/enhance"
)

func newServeCmd(opts *cli.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the webhook receiver and the enhancement reconcile loop",
		RunE:  func(cmd *cobra.Command, _ []string) error { return serve(cmd.Context(), opts) },
	}
	cmd.Flags().String("static-context-file", "",
		"YAML file mapping repositories to owners/attributes (the fake-CMDB enhancer)")
	cmd.Flags().Duration("enhance-grace", 2*time.Minute,
		"how old an opened issue must be before the reconcile pass enhances it")
	return cmd
}

func serve(ctx context.Context, opts *cli.Options) error {
	prov, shutdown, err := telemetry.Init(ctx, telemetry.Config{
		Dir:            os.Getenv("PATCHY_TELEMETRY_DIR"),
		Level:          opts.LogLevel,
		ServiceName:    "context-controller",
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
	chain, err := buildChain(opts.String("static-context-file"))
	if err != nil {
		return err
	}

	ctrl := contextctrl.New(log, clients{resolver},
		contextctrl.Config{Grace: opts.Duration("enhance-grace")}, chain...)
	srv := webhook.NewServer(webhook.Config{Addr: opts.ListenAddr, Secret: secret}, log, ctrl)

	log.LogAttrs(ctx, slog.LevelInfo, "context-controller starting",
		slog.String("addr", opts.ListenAddr),
		slog.Duration("reconcile_interval", opts.ReconcileInterval),
		slog.Int("enhancers", len(chain)))

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return srv.Run(ctx) })
	g.Go(func() error {
		return reconcile.Run(ctx, log, "context-controller", opts.ReconcileInterval, ctrl.Reconcile)
	})
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// buildChain assembles the enhancer chain from config: the static file when
// given, the explicit noop placeholder otherwise.
func buildChain(staticFile string) ([]enhance.Enhancer, error) {
	if staticFile == "" {
		return []enhance.Enhancer{enhancers.Noop{}}, nil
	}
	static, err := enhancers.NewStaticFile(staticFile)
	if err != nil {
		return nil, err
	}
	return []enhance.Enhancer{static}, nil
}

// clients adapts ghclient.Resolver to the controller's Clients seam.
type clients struct{ r ghclient.Resolver }

func (c clients) For(ctx context.Context, repo ghclient.Repo) (contextctrl.Stores, error) {
	return c.r.For(ctx, repo)
}

func (c clients) All(ctx context.Context) ([]contextctrl.Searcher, error) {
	cs, err := c.r.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contextctrl.Searcher, len(cs))
	for i, cl := range cs {
		out[i] = cl
	}
	return out, nil
}
