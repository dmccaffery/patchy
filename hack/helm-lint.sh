#!/bin/sh
# Copyright 2026 Bitwise Media Group Ltd.
# SPDX-License-Identifier: MIT
#
# Lint the helm chart, then render every component combination the templates
# branch on — a broken combination fails here instead of at install time.
set -eu

helm lint helm/chart
helm template patchy helm/chart >/dev/null
helm template patchy helm/chart --set agent.networkPolicy.cilium.enabled=true >/dev/null
helm template patchy helm/chart --set agent.networkPolicy.istio.enabled=true >/dev/null
helm template patchy helm/chart \
  --set agent.networkPolicy.create=false --set agent.createNamespace=false \
  --set sourceController.networkPolicy.create=false \
  --set contextController.networkPolicy.create=false \
  --set remediationController.networkPolicy.create=false >/dev/null
helm template patchy helm/chart \
  --set webhook.host=patchy.example.com \
  --set webhook.ingress.enabled=true --set webhook.ingress.className=nginx \
  --set-json 'webhook.httpRoute={"enabled":true,"parentRefs":[{"name":"gw","namespace":"gateway-system"}]}' >/dev/null
