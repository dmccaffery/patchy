#!/bin/sh
# Copyright 2026 Bitwise Media Group Ltd.
# SPDX-License-Identifier: MIT
#
# One-command local dev loop on Colima: start the VM+k3s if needed, snapshot
# the images, retag the host-arch ones as patchy/<app>:dev (the tags the dev
# overlay expects — the shared Docker/k3s image store makes the tag the whole
# "load" step), apply the overlay, and restart the deployments so a re-run
# redeploys the fresh build. See docs/deployment/colima.md.
set -eu

for tool in colima docker kubectl docker-buildx; do
  command -v "$tool" >/dev/null || {
    echo "error: $tool not found (dotty brew add $tool || brew install $tool)" >&2
    exit 1
  }
done

# start colima with k8s if it isn't running; if it is, trust it and just
# verify the colima kube context answers (catches a colima started without
# --kubernetes). Passing any --k3s-arg replaces colima's default
# [--disable=traefik], so an inert arg (--tls-san=localhost) keeps the
# bundled traefik ingress controller enabled — the dev overlay ships an
# Ingress for the webhook-controller that it satisfies.
colima status >/dev/null 2>&1 || colima start --kubernetes \
  --cpu "${COLIMA_CPU:-4}" --memory "${COLIMA_MEMORY:-8}" \
  --k3s-arg=--tls-san=localhost
kubectl --context colima get --raw /readyz >/dev/null || {
  echo "error: colima is running but k3s is not reachable — start with --kubernetes" >&2
  exit 1
}
kubectl --context colima get ingressclass traefik >/dev/null 2>&1 || {
  echo "note: no traefik IngressClass — this colima predates the traefik-enabled start;" >&2
  echo "      'colima stop' and re-run to reinstall k3s with the ingress controller" >&2
}

mise run snapshot

# retag the host-arch snapshot images as patchy/<app>:dev (dev overlay names)
arch=$(uname -m)
[ "$arch" = x86_64 ] && arch=amd64
for app in webhook-controller source-controller context-controller remediation-controller agent-runner; do
  tag=$(docker images "ghcr.io/bitwise-media-group/patchy/$app" --format '{{.Tag}}' |
    grep -- "-$arch$" | head -1)
  [ -n "$tag" ] || {
    echo "error: no snapshot image for $app ($arch)" >&2
    exit 1
  }
  docker tag "ghcr.io/bitwise-media-group/patchy/$app:$tag" "patchy/$app:dev"
done

kubectl --context colima apply -k deploy/kustomize/overlays/dev

# optional PAT: the dev overlay wires PATCHY_GITHUB_TOKEN (wins over the
# placeholder App creds) from this Secret into the GitHub-facing controllers
if [ -n "${GITHUB_TOKEN:-}" ]; then
  printf '%s' "$GITHUB_TOKEN" |
    kubectl --context colima -n patchy create secret generic patchy-github-token \
      --from-file=token=/dev/stdin --dry-run=client -o yaml |
    kubectl --context colima apply -f -
else
  echo "note: GITHUB_TOKEN not set — controllers will crash-loop on the placeholder" >&2
  echo "      GitHub creds; see docs/deployment/colima.md#github-credentials" >&2
fi

# same :dev tag every run — restart so pods pick up the retagged image
kubectl --context colima -n patchy rollout restart deployment -l app.kubernetes.io/part-of=patchy
kubectl --context colima -n patchy rollout status deployment -l app.kubernetes.io/part-of=patchy --timeout=120s

# where the ingress answers depends on colima's network mode: with a reachable
# VM address colima deliberately does NOT forward guest 80/443 to the host
# (traefik is reached at the VM IP); without one, lima forwards them and
# localhost works. Probe for the webhook route: GET on a POST-only handler
# answering 405 proves the request reached the controller.
addr=$(colima ls --json 2>/dev/null | grep '"name":"default"' | sed -n 's/.*"address": *"\([^"]*\)".*/\1/p')
ingress="http://${addr:-localhost}/webhook"
code=000
for _ in 1 2 3 4 5 6 7 8 9 10; do
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$ingress" || true)
  [ "$code" = "405" ] && break
  sleep 2
done
[ "$code" = "405" ] || echo "note: $ingress answered $code, not the expected 405 — traefik may still be starting" >&2
echo "patchy dev stack ready — webhook ingress: $ingress  nodeport: http://localhost:30079/healthz"
