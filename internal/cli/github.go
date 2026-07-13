// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
)

// GitHub builds the API resolver from the configured auth mode: a personal
// access token when --github-token is set (dev), otherwise the GitHub App
// credentials (production).
func (o *Options) GitHub() (ghclient.Resolver, error) {
	if o.GitHubToken != "" {
		c, err := ghclient.NewToken(o.GitHubToken, o.GitHubBaseURL)
		if err != nil {
			return nil, fmt.Errorf("github token client: %w", err)
		}
		return c, nil
	}
	if o.GitHubAppID == 0 || o.GitHubAppKeyFile == "" {
		return nil, errors.New("github auth: set --github-token, or --github-app-id with --github-app-private-key-file")
	}
	key, err := os.ReadFile(o.GitHubAppKeyFile)
	if err != nil {
		return nil, fmt.Errorf("github app key: %w", err)
	}
	app, err := ghclient.NewApp(ghclient.AppConfig{
		AppID:      o.GitHubAppID,
		PrivateKey: key,
		BaseURL:    o.GitHubBaseURL,
	})
	if err != nil {
		return nil, err
	}
	return app, nil
}
