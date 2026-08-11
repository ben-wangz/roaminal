# Roaminal Helm Chart

This chart deploys Roaminal as one non-root `Deployment` replica with a
`Recreate` strategy. Terminal, SSH, and tmux processes are local to the Pod, so
the chart intentionally does not support horizontal scaling or zero-downtime
updates.

## Prerequisites

- Helm 3.13 or newer (Helm 4 is also supported).
- Kubernetes 1.25 or newer.
- A writable `ReadWriteOnce` storage class, unless `persistence.existingClaim`
  is used.
- An existing Kubernetes Secret containing a stable Roaminal password.

Create the password Secret out of band:

```sh
kubectl -n develop create secret generic roaminal \
  --from-literal=password='use-a-secret'
```

The password is never accepted through values, rendered into a ConfigMap, or
printed by Helm.

## Install

The default chart creates one PVC and stores three logical directories in it:

```text
state/      -> /home/roaminal/.roaminal
workspace/  -> /workspace
ssh/        -> /home/roaminal/.ssh
```

Install from the repository:

```sh
helm upgrade --install roaminal ./chart \
  --namespace develop \
  --create-namespace \
  --set app.acceptTerms=true
```

For an image in a private registry, set `image.registry`, `image.repository`,
`image.tag` or `image.digest`, and `imagePullSecrets` as appropriate.

## Storage

Set `persistence.existingClaim` to reuse a pre-provisioned unified PVC. The
claim must contain the `state/`, `workspace/`, and `ssh/` directories. The
chart keeps chart-created PVCs on uninstall; verify backups before deliberately
deleting the claim.

The container root filesystem is read-only. The chart always mounts a writable
`emptyDir` at `/tmp` for SSH multiplexing sockets; configure its medium and
quota with `tmp.medium` and `tmp.sizeLimit` (the default quota is `64Mi`).

Historical raw manifests used three PVCs. They cannot be adopted directly as
one claim. Stop the old Deployment, back up all three claims, create the new
claim, copy the old contents into the three logical directories, verify the
result, and only then install this chart. Helm never performs that copy or
deletes the old claims. The complete command-by-command procedure, including
rollback and the final manual cleanup, is in
[`docs/deployment.md`](../docs/deployment.md).

To use a read-only SSH Secret instead of the `ssh/` directory in the unified
PVC:

```yaml
ssh:
  source: secret
  secret:
    name: roaminal-ssh
    defaultMode: 256
```

For projected or CSI SSH sources, set `ssh.source: volume`, name the volume in
`ssh.volume.name`, and provide the matching object in `extraVolumes`. Individual
`config`, key, or known-hosts files can be mounted with `ssh.fileMounts`.

Read-only SSH mounts still support reading connections. UI config edits, key
generation/deletion, and known-host updates fail only for the files that are
not writable.

## Ingress and WebSockets

Ingress is disabled by default. When enabled, configure an existing TLS Secret
and controller annotations. The reverse proxy must preserve same-origin
requests and WebSocket upgrades and allow at least 3600 seconds for both read
and send timeouts. The chart does not create certificates or an Ingress
Controller.

## Security and lifecycle

The defaults run UID/GID 1000 with `RuntimeDefault` seccomp, no privilege
escalation, all capabilities dropped, and a read-only root filesystem. The
chart does not create RBAC resources and disables ServiceAccount token
automounting by default. Do not override these settings without reviewing the
security and storage consequences.

An init container creates the three directories on an empty unified PVC. It
does not repair ownership or permissions on an existing mount. Configure the
storage provisioner so UID/GID 1000 can create and use the directories.

Every upgrade uses `Recreate` and interrupts active terminal, SSH, and tmux
processes. Back up the complete unified PVC because it contains SSH material as
well as state and workspace data.

## ForgeKit versions

The repository registry owns the chart version and the linked runtime image
version. Use ForgeKit to inspect or bump them; do not edit `Chart.yaml` version,
`Chart.yaml` appVersion, or `values.yaml` image tags by hand.
