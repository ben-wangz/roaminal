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

## Kubernetes

The manifests in `deploy/kubernetes/` are intentionally ordinary YAML. Replace
the image in `deployment.yaml` with the pushed Git-SHA tag and create the Secret
from a secret manager rather than applying `secret.example.yaml` unchanged.
The Deployment has one replica and `Recreate`, separate RWO state/workspace/SSH PVCs, a
ClusterIP Service, restrictive security contexts, and the required startup,
readiness, and liveness probes. State is mounted at
`/home/roaminal/.roaminal`, SSH facts at `/home/roaminal/.ssh`, and the local
connection's initial cwd is `/workspace`.

The SSH volume can instead be a read-only whole-directory Secret, or a direct
Secret mount at `config` or an allowlisted key filename. Read-only mounts remain
usable for local and remote connections; only structured config edits, key
generation, and OpenSSH known-host updates lose their write capability.

```sh
kubectl apply --server-side --dry-run=server -n develop \
  -f deploy/kubernetes/configmap.yaml \
  -f deploy/kubernetes/pvc.yaml \
  -f deploy/kubernetes/service.yaml \
  -f deploy/kubernetes/deployment.yaml
kubectl -n develop create secret generic roaminal \
  --from-literal=password='use-a-secret' --dry-run=client -o yaml | kubectl apply -f -
kubectl -n develop apply -f deploy/kubernetes/configmap.yaml -f deploy/kubernetes/pvc.yaml \
  -f deploy/kubernetes/service.yaml -f deploy/kubernetes/deployment.yaml
kubectl -n develop rollout status deployment/roaminal --timeout=180s
```

For direct in-cluster verification use
`http://roaminal.develop.svc.cluster.local:9846`. Do not use `kubectl port-forward`;
the Playwright Kubernetes gate sets the secure-context exception only for this exact
HTTP origin. Production traffic must use the TLS ingress below.

Use a TLS-aware ingress or reverse proxy in front of the Service. Terminate TLS
there, preserve the original same-origin host, and configure WebSocket upgrade
plus at least 3600 seconds for both read and send timeouts. Do not expose the
worker, PVCs, or a runtime socket.

PVCs must be writable by UID/GID 1000 (or use an equivalent storage policy that
sets the ownership). Keep `fsGroup: 1000` when the storage driver supports it.
The mounted `.ssh` directory must be a direct directory owned by `root:1000` or
`1000:1000`, with no world-write bit (for example mode `2770` or `0700` when
the runtime user owns it). Roaminal intentionally does not repair ownership or
permissions. Some local-path provisioners create a new mount root as `0777`;
configure the provisioner or initialize that empty PVC before the application
rollout. Do not apply this step to a read-only Secret mount.
Keep the SSH PVC separate from state backups and treat it as high-sensitivity
credential material. Back up the state and SSH volumes before upgrades.
`Recreate` intentionally interrupts the
single service during an upgrade; the new pod starts new Bash processes and
restores metadata and scrollback from the state PVC. See
[backup/recovery](backup-recovery.md).
