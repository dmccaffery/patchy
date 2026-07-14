# Local development on Colima

The [dev overlay](kustomize.md) assumes a local [kind](https://kind.sigs.k8s.io/) cluster, but
[Colima](https://github.com/abiosoft/colima) — a Lima VM running Docker with an optional embedded [k3s](https://k3s.io/)
— is a drop-in alternative on macOS (and Linux). The same overlay applies unchanged, and three things get simpler:

- **No image loading.** With Colima's default Docker runtime, k3s shares Docker's image store — anything you
  `docker build` or `docker tag` is immediately runnable in the cluster. The `kind load docker-image` step disappears.
- **No port-mapping config.** Colima forwards every listening TCP port in the VM to `127.0.0.1` on the host
  automatically, so the dev overlay's NodePorts (30079–30082) — and an ingress controller's 80/443 — appear on
  `localhost` without kind's `extraPortMappings`.
- **NetworkPolicy is enforced.** k3s embeds a network policy controller, so the base default-deny in `patchy-agents`
  actually applies — unlike kindnet, which accepts the policies and ignores them.

That last point is also why Colima supports the piece kind makes awkward: a real **Ingress in front of the
webhook-controller**, the same shape production uses, instead of a bare NodePort.

## Start the cluster

```sh
brew install colima kubectl
colima start --kubernetes --cpu 4 --memory 8
```

`--kubernetes` boots k3s inside the VM (pin it with `--kubernetes-version`, which must match a k3s release tag) and
switches your Docker and kubeconfig contexts to `colima`. Two k3s defaults matter here:

- Colima passes `--disable=traefik` to k3s, so the bundled Traefik ingress controller is **not** running — install
  ingress-nginx below (or override `--k3s-arg` at start time if you prefer the bundled Traefik and
  `className: traefik`).
- k3s's `servicelb` (klipper-lb) **is** running: a `LoadBalancer` Service gets the VM's node address and binds its ports
  on the node, which is what lets Colima's port forwarding put an ingress controller on `localhost`.

## Build the images

`make snapshot` builds per-arch, unpushed `ghcr.io/…:v<next>-snapshot-<sha>-<arch>` images. Retag the host-arch ones
with the `patchy/<name>:dev` names the dev overlay expects — and that is the whole "load" step, because the Docker
runtime and k3s share one image store:

```sh
make snapshot

arch=arm64 # amd64 on Intel
for app in webhook-controller source-controller context-controller \
           remediation-controller agent-runner; do
  tag=$(docker images "ghcr.io/bitwise-media-group/patchy/$app" \
    --format '{{.Tag}}' | grep -- "-$arch$" | head -1)
  docker tag "ghcr.io/bitwise-media-group/patchy/$app:$tag" "patchy/$app:dev"
done
```

The `dev` tag never hits a registry: it is not `:latest`, so the pull policy defaults to `IfNotPresent` and k3s runs the
local image as-is.

!!! note "Containerd runtime"

    If you started Colima with `--runtime containerd`, only images in containerd's `k8s.io` namespace are visible to
    Kubernetes — build or import with `nerdctl --namespace k8s.io`. The Docker runtime avoids the extra step.

## Apply the dev overlay

```sh
kubectl apply -k deploy/kustomize/overlays/dev
```

Everything the [Kustomize page](kustomize.md) says about dev applies — placeholder Secrets, 2-minute windows, the fake
harness — and the NodePorts are reachable immediately, no cluster config needed:

```sh
curl -s http://localhost:30079/healthz   # webhook-controller, the routing entry point
```

At this point you can stop and use the kind flow verbatim: point a tunnel (`gh webhook forward`, smee.io) at
`http://localhost:30079/webhook`.

## Ingress for the webhook-controller

The NodePort works, but Colima can also run the production shape: an ingress controller in front of the
`patchy-webhook-controller` Service, exposing only `/webhook`. Install ingress-nginx — the class the examples across
these docs use:

```sh
helm upgrade --install ingress-nginx ingress-nginx \
  --repo https://kubernetes.github.io/ingress-nginx \
  --namespace ingress-nginx --create-namespace
```

servicelb gives its `LoadBalancer` Service the node's address, and Colima forwards the listening 80/443 to `localhost`
(macOS allows unprivileged binds below 1024, so no sudo is involved).

Then put an Ingress in front of the webhook-controller. With the **dev overlay**, apply one directly (or add it to an
overlay of your own) — host-less, so plain `localhost` matches:

```yaml
# webhook-ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: patchy-webhook
  namespace: patchy
spec:
  ingressClassName: nginx
  rules:
    - http:
        paths:
          - path: /webhook
            pathType: Prefix
            backend:
              service:
                name: patchy-webhook-controller
                port:
                  number: 8080
```

With the **Helm chart**, use the built-in flavour instead — `webhook.host` is required, and an
[sslip.io](https://sslip.io) name resolves to `127.0.0.1` without touching `/etc/hosts`:

```yaml
webhook:
  host: patchy.127.0.0.1.sslip.io
  ingress:
    enabled: true
    className: nginx
    # no tls locally — the tunnel below terminates GitHub's HTTPS leg
```

Smoke-test the route. The webhook server registers `POST /webhook` only, so a GET answering **405** proves the request
reached the controller (a 404 means the Ingress didn't match):

```sh
curl -i http://localhost/webhook                      # dev-overlay Ingress
curl -i http://patchy.127.0.0.1.sslip.io/webhook      # chart Ingress
```

Finally, tunnel GitHub deliveries at the Ingress instead of the NodePort:

```sh
gh webhook forward --repo <owner>/<repo> \
  --events code_scanning_alert,issues,issue_comment,pull_request \
  --url http://localhost/webhook
```

TLS stays out of the local picture on purpose: GitHub's HTTPS leg ends at the tunnel, which re-delivers to the Ingress
over plain HTTP on your machine. If you want the cluster reachable from other devices instead of a tunnel, start Colima
with `--network-address` to give the VM a routable IP and point clients (and an sslip.io name built from that IP) at it.

!!! warning "Closer to production, still not a sandbox"

    k3s enforcing NetworkPolicy means the L3/L4 floor from the [isolation model](isolation.md) is real on Colima —
    an improvement over kind, where it silently does nothing. The FQDN layer still isn't there (no Cilium, no
    Istio), and the dev overlay ships throwaway credentials and the fake harness. Treat this as a faster inner
    loop, not a security-representative environment.
