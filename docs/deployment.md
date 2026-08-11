# Deployment

## Published artifacts

GitHub Actions publishes release artifacts from `roaminal-v<chart-version>` tags.
Deployment users consume them; they do not build or push release artifacts.

| Artifact | Reference |
| --- | --- |
| Runtime image | `ghcr.io/ben-wangz/roaminal:<runtime-version>` |
| Helm Chart | `oci://ghcr.io/ben-wangz/roaminal-charts/roaminal:<chart-version>` |

Select the Chart version. Its `appVersion` and default image tag are synchronized
with the linked runtime image. Maintainers use [release automation](releasing.md).

## Local container

Use Podman only for local integration; do not push this image as a release.

```sh
IMAGE=roaminal:dev-$(git rev-parse --short HEAD)
podman build --file container/Containerfile --tag "$IMAGE" .
podman run --rm --name roaminal -p 9846:9846 \
  -e ROAMINAL_ACCEPT_TERMS=true -e ROAMINAL_PASSWORD='use-a-secret' \
  -v roaminal-state:/home/roaminal/.roaminal \
  -v roaminal-ssh:/home/roaminal/.ssh \
  -v roaminal-workspace:/workspace "$IMAGE"
```

## Helm install

Helm 3.13+ and Kubernetes 1.25+ are required. Set `GHCR_USERNAME` and
`GHCR_TOKEN` only for a private package.

```sh
export ROAMINAL_CHART_VERSION='<chart-version>'
export ROAMINAL_CHART_REF='oci://ghcr.io/ben-wangz/roaminal-charts/roaminal'
printf '%s' "$GHCR_TOKEN" | helm registry login ghcr.io \
  --username "$GHCR_USERNAME" --password-stdin

kubectl -n develop create secret generic roaminal \
  --from-literal=password='use-a-secret' \
  --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install roaminal "$ROAMINAL_CHART_REF" \
  --version "$ROAMINAL_CHART_VERSION" \
  --namespace develop --create-namespace \
  --values deploy/kubernetes/values.yaml \
  --set auth.existingSecret=roaminal \
  --set app.acceptTerms=true
kubectl -n develop rollout status deployment/roaminal --timeout=180s
```

The repository values file is intentionally empty until an override is needed.
The Chart creates one RWO PVC with `state/`, `workspace/`, and `ssh/` subpaths.
Use `persistence.existingClaim` for a prepared unified claim. The chart retains
its PVC on uninstall.

Read-only SSH Secrets and projected volumes remain usable for connections, but
SSH config edits and key operations require a writable source. State and
workspace must always be writable by UID/GID 1000.

## Legacy PVC migration

The retired raw manifests used three PVCs. Back up those claims, stop the old
Deployment, create a unified claim, and copy each old claim into `state/`,
`workspace/`, or `ssh/` with a temporary root-owned copier Pod. Install the
Chart with `persistence.existingClaim`, verify login and local/SSH connections,
then remove old PVCs only after a backup and acceptance period. Helm does not
copy data, adopt old claims, or undo this migration.

## Network and validation

Run `helm template` with the selected OCI Chart and apply a server-side dry run
before production changes. The Service is ClusterIP; expose it through a TLS
reverse proxy that preserves WebSocket upgrades and uses read/send timeouts of
at least one hour. Do not expose the worker or runtime sockets. For in-cluster
tests use the Helm Service URL directly, never a port-forward.
