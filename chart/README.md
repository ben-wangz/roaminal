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

## Database

SQLite is the default and requires `persistence.enabled: true`. Roaminal
stores it at `/home/roaminal/.roaminal/db/roaminal.sqlite3` on the state PVC.
Before startup the init container restores private modes on the state and SQLite
`db/` directories, the database and its sidecars, and the local binding. These
paths must be owned by the Roaminal UID (1000 by default); symlinks and multiply
linked files are rejected. The init container does not repair ownership,
change legacy files, or recursively chmod the PVC.
The whole state directory is mounted so SQLite sidecars stay with the DB. Use
SQLite-compatible storage and take SQLite-aware backups or consistent offline
snapshots, not copies of just the live database file.

The chart sets `ROAMINAL_DB_DRIVER`, `ROAMINAL_DB_SQLITE_PATH`, and
`ROAMINAL_DB_POSTGRES_DSN` explicitly, with an empty inactive setting. Do not
set DB env via `extraEnv` or `extraEnvFrom`. Use a Roaminal image that supports
these settings, `db prepare-state`, and `/readyz`. Startup and liveness still
use `/healthz`. Keep
more than 60 seconds for PostgreSQL startup retries when tuning the startup
probe (the default window is 120 seconds).

### External PostgreSQL

Provision a database and a non-superuser role. Put the full PostgreSQL URI
(endpoint, database, user, password, and TLS options) in a required Secret in
the release namespace, never in Helm values or a ConfigMap. Use verified TLS
(`sslmode=verify-full`). No CloudNativePG resources or bootstrap RBAC are
created in this mode.

```yaml
database:
  driver: postgres
  postgres:
    existingSecret: roaminal-postgres
    dsnKey: dsn
    # Optional: omit this block if the URI uses system trust.
    caSecret:
      name: postgres-ca
      key: ca.crt
```

If specified, the CA Secret must also be in the release namespace. Its selected
key is mounted read-only at `/etc/roaminal/postgres-ca/ca.crt`; reference that
path with `sslrootcert` in the URI. Secret updates do not change a running
container's env. Coordinate database role rotation, Secret updates, and a
controlled restart. PostgreSQL stores database records, not the local database
binding, workspace files, or native SSH files.

### Managed PostgreSQL

Install the CloudNativePG Operator and `Cluster` CRD separately and pin their
version. The reference version is [CloudNativePG v1.30.2][cnpg]; it supports
Kubernetes 1.34–1.36. SQLite and external PostgreSQL do not need this Operator.

```yaml
database:
  driver: postgres
  postgres:
    managed:
      storage:
        size: 10Gi
        storageClass: "" # Use the default class.
```

Leave `database.postgres.existingSecret` empty. The chart declares one
CloudNativePG instance and a separate database PVC. The credential Job runs
`roaminal db bootstrap-postgres` in the same pinned image as the application.
It creates a release-qualified `kubernetes.io/basic-auth` Secret with
`username`, `password`, and the URI under `database.postgres.dsnKey` (default
`dsn`). The `Cluster` uses that Secret for `initdb`; the app uses the URI for
`<cluster>-rw.<namespace>.svc` with verified TLS. The app mounts only `ca.crt`
from the Operator-generated `<cluster>-ca` Secret.

The Job is not a Helm hook. Argo CD sync waves run the bootstrap identity and
RBAC at `-2`, the Job at `-1`, and the Cluster and Deployment at `0`. The Job
does not wait for PostgreSQL: it must complete before the Cluster is applied.
Helm does not use Argo waves; required Secret references block startup until
credentials and CA exist, and the app retries startup against the same URI.
Use `helm upgrade --install --wait --wait-for-jobs --timeout 15m` with the
release, chart, namespace and values, increasing the timeout for slow volumes
or image pulls. A connection failure never triggers password regeneration.

The bootstrap Job uses a dedicated ServiceAccount and a short-lived projected
API token. It can `get` only the named credential Secret and marker ConfigMap,
and `create` Secrets and ConfigMaps in the release namespace. Kubernetes RBAC
cannot constrain `create` by name. Use an admission policy matching this
ServiceAccount on CREATE to restrict it to the exact rendered names (no
`generateName`), a `kubernetes.io/basic-auth` Secret, and an empty marker
ConfigMap. The app Pods do not automount API tokens.

The Job name changes only when its spec or
`database.postgres.managed.bootstrapRevision` changes. The Secret and marker
names stay stable, and the Job never overwrites a Secret. Treat the managed
database name, username, and DSN key as part of the installation identity, not
routine upgrade settings. Back up the database, credential Secret, and marker
together. The Secret and marker are created at
runtime and are not owned by the Job or tracked by Helm or Argo CD. The Cluster
is marked to keep on Helm removal and not to prune or delete in Argo CD.
CloudNativePG owns its PVCs, so deleting the Cluster may garbage-collect them;
verify the PV reclaim policy (use `Retain` if required) and keep verified
backups before deleting the Cluster or namespace.

If the Secret is missing but the marker remains, bootstrap waits for the
Secret and fails closed. Pause reconciliation before recovery. With initialized
storage, restore the matching Secret and marker or explicitly reset the DB role
and restore matching credentials; do not clear the guard and generate a new
password against those volumes. If both objects are lost, recover them before
reinstalling against retained data. Clear a stranded marker and bump
`bootstrapRevision` only after verifying PostgreSQL never initialized.

[cnpg]: https://github.com/cloudnative-pg/cloudnative-pg/releases/tag/v1.30.2

## Storage and maintenance

Both drivers require `persistence.enabled: true`. Even with PostgreSQL, the
unified PVC retains the local database binding, workspace, and default SSH
storage, independently of the PostgreSQL PVC. Set `persistence.existingClaim`
to reuse a claim. The init container creates missing directories if the claim
is writable by UID/GID 1000. The chart keeps chart-created PVCs on uninstall;
verify backups before deliberately deleting the claim.

Set `maintenance.enabled: true` to scale only the Roaminal Deployment to zero
without removing storage or database resources. Wait for the old Pod to exit
before an offline backup or migration. Changing drivers does not migrate data;
complete and verify the offline import before disabling maintenance.

The container root filesystem is read-only. The chart always mounts a writable
`emptyDir` at `/tmp` for SSH multiplexing sockets; configure its medium and
quota with `tmp.medium` and `tmp.sizeLimit` (the default quota is `64Mi`).

Image previews use a second writable `emptyDir` at the fixed path
`/var/cache/roaminal/filesystem-image-previews`. Configure its quota with
`filesystemImagePreview.cache.emptyDir.sizeLimit` (default `192Mi`) and its
storage medium with `filesystemImagePreview.cache.emptyDir.medium`. The
default quota is `128Mi` cache target plus one `32Mi` source, one `16Mi`
encoded output, and `16Mi` overhead. Operators increasing preview concurrency
or byte limits must increase the quota using
`cache target + maxConversions * (maxSource + maxOutput) + 16Mi`.

The remaining `filesystemImagePreview` fields mirror the backend limits and
are rendered through the ConfigMap. The fixed mount path is not overridable,
and this cache is never placed on the state PVC, workspace, SSH volume, or
`/tmp`. Volume exhaustion degrades only image derivative generation.

Older multi-volume deployments used three PVCs. They cannot be adopted directly
as one claim. Stop the old Deployment, back up all three claims, create the new
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

Client diagnostics are enabled by default. Set
`app.clientDiagnosticsEnabled=false` to disable browser reports. When enabled,
redacted events use the existing `state/` subdirectory and are capped at five
files and 10 MiB; no extra PVC is created.

## Remote browser

Remote Browser is disabled by default. Enable it only when the Pod is allowed
to run the bundled non-root Chromium worker:

```yaml
app:
  browserEnabled: true
  browserWorkerPath: /opt/roaminal/browser-worker/index.mjs
  browserChromiumPath: /usr/bin/chromium
```

Chromium requests services from inside the Roaminal Pod, so `localhost` and
cluster DNS names resolve from that Pod. The feature does not inherit routes
from an SSH connection instance. The worker's CDP endpoint is loopback-only;
the Chart exposes only the existing Roaminal Service.

## Browser notifications

Web Push is disabled by default. To enable the authenticated sender, create an
existing Secret with the VAPID public key, private key, and subject, then set:

```yaml
webPush:
  existingSecret: roaminal-web-push
  vapidPublicKeyKey: public-key
  vapidPrivateKeyKey: private-key
  subjectKey: subject
```

The browser's single notification switch controls both foreground and Web Push
delivery for the current login session. The chart never creates or renders the
Web Push Secret.

## Ingress and WebSockets

Ingress is disabled by default. When enabled, configure an existing TLS Secret
and controller annotations. The reverse proxy must preserve same-origin
requests and WebSocket upgrades and allow at least 3600 seconds for both read
and send timeouts. The chart does not create certificates or an Ingress
Controller.

## Security and lifecycle

The defaults run UID/GID 1000 with `RuntimeDefault` seccomp, no privilege
escalation, all capabilities dropped, and a read-only root filesystem. The app
does not automount ServiceAccount tokens. Only managed PostgreSQL adds
credential RBAC. Configure the storage provisioner so UID/GID 1000 can create
and use the directories; the init container does not repair ownership.

The single serving replica uses `Recreate`; replacing it interrupts live
connection instances even with PostgreSQL. Back up the complete unified PVC
because it contains SSH material as well as state and workspace data.

## ForgeKit versions

The repository registry owns the chart version and the linked runtime image
version. Use ForgeKit to inspect or bump them; do not edit `Chart.yaml` version,
`Chart.yaml` appVersion, or `values.yaml` image tags by hand.

Published artifacts are available from GitHub Container Registry:

```sh
helm registry login ghcr.io
helm install roaminal \
  oci://ghcr.io/ben-wangz/roaminal-charts/roaminal \
  --version <chart-version> \
  --namespace roaminal --create-namespace \
  --set app.acceptTerms=true \
  --set auth.existingSecret=roaminal-auth
```

The default image is `ghcr.io/ben-wangz/roaminal`. Override `image.registry`,
`image.repository`, or `image.digest` for a private mirror or an immutable
deployment policy.
