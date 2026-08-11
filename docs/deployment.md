# Deployment

## Podman

Build an immutable Git-SHA image, then run it with persistent state and
workspace volumes. The runtime image contains the Go service, Node runtime,
official xterm headless worker, Bash, CA certificates, OpenSSH client, and
`tini`; it does not contain npm, compilers, or an SSH server.

```sh
IMAGE=registry.internal.example/roaminal:$(git rev-parse HEAD)
podman build --file container/Containerfile --tag "$IMAGE" .
podman run --rm --name roaminal \
  --restart unless-stopped -p 9846:9846 \
  -e ROAMINAL_ACCEPT_TERMS=true -e ROAMINAL_PASSWORD='use-a-secret' \
  -v roaminal-state:/home/roaminal/.roaminal \
  -v roaminal-ssh:/home/roaminal/.ssh \
  -v roaminal-workspace:/workspace "$IMAGE"
podman push "$IMAGE"
```

Use `podman run --rm` without `--restart` for an integration harness that owns
the process lifecycle. A port conflict is an error; Roaminal never selects a
different port. Verify `GET /healthz` before exercising login or WebSocket flow.

## Kubernetes with Helm

The source of truth is the self-contained Chart in `chart/`. It requires Helm
3.13 or newer (Helm 4 is supported) and Kubernetes 1.25 or newer. Create the
password Secret through a secret manager or an equivalent out-of-band process:

```sh
kubectl -n develop create secret generic roaminal \
  --from-literal=password='use-a-secret' \
  --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install roaminal ./chart \
  --namespace develop \
  --create-namespace \
  --set app.acceptTerms=true
kubectl -n develop rollout status deployment/roaminal --timeout=180s
```

The Chart creates one unified RWO PVC. Its fixed logical directories are mounted
as `/home/roaminal/.roaminal`, `/workspace`, and `/home/roaminal/.ssh`. Set
`persistence.existingClaim` to use an already prepared unified claim. Historical
raw manifests used three PVCs; for such an installation, stop the old
Deployment, back up all three, create the unified claim, copy their contents
into `state/`, `workspace/`, and `ssh/`, verify the result, and only then install
Helm. Helm does not perform or undo this migration. The former manifests are
retired; `deploy/kubernetes/README.md` remains as a migration pointer.

### Explicit migration from the historical manifests

The following is an operator-run, one-way data copy. Replace the image and
capacity with values appropriate for the cluster. Capture the old objects and
take a storage-level backup before changing the Deployment:

```sh
kubectl -n develop get deployment/roaminal service/roaminal configmap/roaminal -o yaml \
  > /secure-backup/roaminal-legacy-resources.yaml
kubectl -n develop scale deployment/roaminal --replicas=0
kubectl -n develop wait --for=delete pod -l app.kubernetes.io/name=roaminal --timeout=180s
```

Create the target claim without asking Helm to adopt it yet. The chart's
`helm.sh/resource-policy: keep` annotation is intentional:

```sh
helm template roaminal ./chart --namespace develop \
  --show-only templates/pvc.yaml \
  --set persistence.size=16Gi | kubectl apply -f -
kubectl -n develop wait --for=jsonpath='{.status.phase}'=Bound \
  pvc/roaminal-data --timeout=180s
```

Run a temporary root copy Pod so ownership can be set without weakening the
application Pod. The old claims are mounted read-only and are never modified:

```sh
kubectl -n develop apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: roaminal-data-migration
spec:
  restartPolicy: Never
  securityContext:
    runAsUser: 0
    runAsGroup: 0
  containers:
    - name: copy
      image: registry.example.invalid/roaminal:REPLACE_WITH_GIT_SHA
      command: ["/bin/sh", "-ec"]
      args:
        - |
          set -eu
          mkdir -p /new/state /new/workspace /new/ssh
          cp -a /old-state/. /new/state/
          cp -a /old-workspace/. /new/workspace/
          cp -a /old-ssh/. /new/ssh/
          chown -R 1000:1000 /new/state /new/workspace /new/ssh
          chmod 0700 /new/state /new/workspace /new/ssh
          echo 'migration complete'
      volumeMounts:
        - {name: old-state, mountPath: /old-state, readOnly: true}
        - {name: old-workspace, mountPath: /old-workspace, readOnly: true}
        - {name: old-ssh, mountPath: /old-ssh, readOnly: true}
        - {name: new-data, mountPath: /new}
  volumes:
    - {name: old-state, persistentVolumeClaim: {claimName: roaminal-state}}
    - {name: old-workspace, persistentVolumeClaim: {claimName: roaminal-workspace}}
    - {name: old-ssh, persistentVolumeClaim: {claimName: roaminal-ssh}}
    - {name: new-data, persistentVolumeClaim: {claimName: roaminal-data}}
YAML
kubectl -n develop wait --for=jsonpath='{.status.phase}'=Succeeded \
  pod/roaminal-data-migration --timeout=300s
kubectl -n develop logs pod/roaminal-data-migration
kubectl -n develop delete pod roaminal-data-migration
```

Install the chart against the prepared claim and verify all three directories,
the password Secret, login, and at least one local/SSH connection before
considering the migration complete:

```sh
helm upgrade --install roaminal ./chart --namespace develop \
  --set persistence.existingClaim=roaminal-data \
  --set auth.existingSecret=roaminal --set app.acceptTerms=true
kubectl -n develop rollout status deployment/roaminal --timeout=180s
kubectl -n develop exec deployment/roaminal -- \
  find /home/roaminal/.roaminal /workspace /home/roaminal/.ssh -maxdepth 1 -mindepth 1 -print
```

If the chart rollout fails, run `helm uninstall roaminal --namespace develop`
(the unified PVC is retained), restore the captured legacy objects, and scale
the old Deployment back to one replica. Do not delete or
overwrite the three source PVCs until the new release has passed its data and
connection checks. After a successful acceptance window, remove the old PVCs
manually and only after a final backup:

```sh
kubectl -n develop delete pvc roaminal-state roaminal-workspace roaminal-ssh
```

The SSH source can be switched to a read-only whole-directory Secret or a
projected/CSI volume. Individual `config`, key, or known-hosts files can be
mounted with `ssh.fileMounts`. Read-only mounts remain usable for local and
remote connections; only the operations requiring writes lose their capability.

Validate before applying:

```sh
helm lint ./chart --strict
helm template roaminal ./chart --namespace develop > /tmp/roaminal.yaml
kubectl apply --server-side --dry-run=server -f /tmp/roaminal.yaml
```

For direct in-cluster verification use
`http://roaminal.develop.svc.cluster.local:9846`. Do not use `kubectl port-forward`;
the Playwright Kubernetes gate sets the secure-context exception only for this exact
HTTP origin. Production traffic must use the TLS ingress below.

Use a TLS-aware ingress or reverse proxy in front of the Service. Terminate TLS
there, preserve the original same-origin host, and configure WebSocket upgrade
plus at least 3600 seconds for both read and send timeouts. Do not expose the
worker, PVCs, or a runtime socket.

The unified PVC must be writable by UID/GID 1000 (or use an equivalent storage
policy that sets ownership). The init container creates the three logical
directories on an empty claim but intentionally does not repair an existing
mount's ownership or permissions. A read-only Secret/projected SSH source does
not need a writable PVC `ssh/` directory, but state and workspace still do.
The chart always mounts a writable `emptyDir` at `/tmp` for runtime sockets;
configure its quota with `tmp.sizeLimit` when the default `64Mi` is not enough.
Treat the complete unified PVC as high-sensitivity credential material because
it contains SSH data as well as state and workspace. Back it up before upgrades.
`Recreate` intentionally interrupts the single service during an upgrade; the
new Pod starts new Bash processes and restores metadata and scrollback from the
state directory. See
[backup/recovery](backup-recovery.md).
