// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

// Package ghsecret turns credential Secrets referenced by Forge and
// Integration resources into GitHub clients. One Secret shape serves both
// kinds: a PAT under "token" (dev), or a GitHub App under "appID" +
// "privateKey" (production), with "webhookSecret" carrying the receiver HMAC
// secret where relevant. Apps are memoized per Secret resourceVersion so
// rotation transparently rebuilds while steady state costs nothing.
package ghsecret

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"

	"github.com/bitwise-media-group/patchy/internal/ghclient"
)

// Secret keys.
const (
	// KeyToken is a PAT — the dev fallback; production uses App auth.
	KeyToken = "token"
	// KeyAppID is the GitHub App ID (decimal string).
	KeyAppID = "appID"
	// KeyPrivateKey is the App's PEM-encoded RSA private key.
	KeyPrivateKey = "privateKey"
	// KeyWebhookSecret is the receiver HMAC secret.
	KeyWebhookSecret = "webhookSecret"
)

// Token returns the PAT held in the secret, if any.
func Token(secret *corev1.Secret) (string, bool) {
	tok := strings.TrimSpace(string(secret.Data[KeyToken]))
	return tok, tok != ""
}

// Apps memoizes ghclient.App instances per credential Secret.
type Apps struct {
	mu   sync.Mutex
	apps map[string]*ghclient.App
}

// NewApps builds an empty memo.
func NewApps() *Apps {
	return &Apps{apps: make(map[string]*ghclient.App)}
}

// FromSecret builds (or returns the memoized) App from the secret's
// appID/privateKey keys. baseURL points at GHES; empty means github.com.
func (a *Apps) FromSecret(secret *corev1.Secret, baseURL string) (*ghclient.App, error) {
	cacheKey := secret.Namespace + "/" + secret.Name + "/" + secret.ResourceVersion
	a.mu.Lock()
	defer a.mu.Unlock()
	if app, ok := a.apps[cacheKey]; ok {
		return app, nil
	}
	rawID := strings.TrimSpace(string(secret.Data[KeyAppID]))
	if rawID == "" {
		return nil, fmt.Errorf("secret %s/%s has neither %s nor %s",
			secret.Namespace, secret.Name, KeyToken, KeyAppID)
	}
	appID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("secret key %s: %w", KeyAppID, err)
	}
	key := secret.Data[KeyPrivateKey]
	if len(key) == 0 {
		return nil, fmt.Errorf("secret %s/%s missing key %s", secret.Namespace, secret.Name, KeyPrivateKey)
	}
	app, err := ghclient.NewApp(ghclient.AppConfig{AppID: appID, PrivateKey: key, BaseURL: baseURL})
	if err != nil {
		return nil, err
	}
	// Drop stale versions of this secret so rotation doesn't grow the map.
	prefix := secret.Namespace + "/" + secret.Name + "/"
	for k := range a.apps {
		if strings.HasPrefix(k, prefix) {
			delete(a.apps, k)
		}
	}
	a.apps[cacheKey] = app
	return app, nil
}

// Validate checks the secret is usable: a non-empty PAT or a parseable App
// credential.
func (a *Apps) Validate(secret *corev1.Secret, baseURL string) error {
	if _, ok := secret.Data[KeyToken]; ok {
		if _, nonEmpty := Token(secret); !nonEmpty {
			return errors.New("secret key token is empty")
		}
		return nil
	}
	_, err := a.FromSecret(secret, baseURL)
	return err
}
