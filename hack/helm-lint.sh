#!/bin/sh
# Copyright 2026 Bitwise Media Group Ltd.
# SPDX-License-Identifier: MIT
#
# Lint both helm charts, then render every component combination the templates
# branch on — a broken combination fails here instead of at install time.
set -eu

helm lint charts/patchy
helm template patchy charts/patchy >/dev/null
helm template patchy charts/patchy --set agent.networkPolicy.cilium.enabled=true >/dev/null
helm template patchy charts/patchy --set agent.networkPolicy.istio.enabled=true >/dev/null
helm template patchy charts/patchy \
  --set agent.networkPolicy.create=false --set agent.createNamespace=false \
  --set sourceController.networkPolicy.create=false \
  --set contextController.networkPolicy.create=false \
  --set remediationController.networkPolicy.create=false >/dev/null
helm template patchy charts/patchy \
  --set webhook.host=patchy.example.com \
  --set webhook.ingress.enabled=true --set webhook.ingress.className=nginx \
  --set-json 'webhook.httpRoute={"enabled":true,"parentRefs":[{"name":"gw","namespace":"gateway-system"}]}' >/dev/null
helm template patchy charts/patchy --set statusServer.enabled=false >/dev/null
helm template patchy charts/patchy \
  --set statusServer.host=patchy-status.example.com \
  --set statusServer.ingress.enabled=true --set statusServer.ingress.className=nginx \
  --set-json 'statusServer.httpRoute={"enabled":true,"annotations":{},"parentRefs":[{"name":"gw","namespace":"gateway-system"}]}' \
  --set statusServer.rbac.userRoles=true \
  --set-json 'statusServer.auth.config={"mode":"oidc","oidc":{"issuerURL":"https://sso.example.com","clientID":"patchy-status","clientSecret":"placeholder"}}' >/dev/null

# The CR chart: the empty default plus a populated render of both arrays.
helm lint charts/patchy-config
helm template patchy-config charts/patchy-config >/dev/null
helm template patchy-config charts/patchy-config \
  --set-json 'integrations=[{"name":"github","spec":{"provider":"github","secretRef":{"name":"patchy-github"},"interval":"10m"}}]' \
  --set-json 'forges=[{"name":"github","spec":{"provider":"github","secretRef":{"name":"patchy-github"},"interval":"10m"}}]' >/dev/null
