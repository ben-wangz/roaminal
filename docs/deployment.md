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

## Podman

For a single-host run using a published runtime image, see
[Podman usage](podman/usage.md). The build below is for local integration; do
not push this image as a release.

```sh
IMAGE=roaminal:dev-$(git rev-parse --short HEAD)
podman build --file container/Containerfile --tag "$IMAGE" .
podman run --rm --name roaminal -p 9846:9846 \
  -e ROAMINAL_ACCEPT_TERMS=true -e ROAMINAL_PASSWORD='use-a-secret' \
  -v roaminal-state:/home/roaminal/.roaminal \
  -v roaminal-ssh:/home/roaminal/.ssh \
  -v roaminal-image-previews:/var/cache/roaminal/filesystem-image-previews \
  -v roaminal-workspace:/workspace "$IMAGE"
```

The root `.containerignore` excludes Git metadata, dependency directories,
frontend build output, test reports, and logs from the build context. The image
contains the compiled Agent component bundle under
`/opt/roaminal/agents/hooks/codex`; do not depend on a host checkout at
runtime.

## Kubernetes with Helm

Helm 3.13+ and Kubernetes 1.25+ are required; managed PostgreSQL has additional
prerequisites. For a fresh install, use the [published Chart](../chart/README.md#forgekit-versions)
and its compatible database-enabled image. Pass
`--values deploy/kubernetes/values.yaml` if using the repository overrides.
For an upgrade from file-backed state, [complete the offline cutover](#cut-over-from-legacy-files)
before starting the database-enabled image.

The Chart runs one serving replica with `Recreate`; PostgreSQL does not make
live connection instances highly available. Both drivers require the unified
RWO PVC (`persistence.enabled: true`) with `state/`, `workspace/`, and `ssh/`
subpaths. Use `persistence.existingClaim` for a prepared claim. The Chart retains
its own Roaminal PVC on uninstall.

- **SQLite (default):** `/home/roaminal/.roaminal/db/roaminal.sqlite3` on the
  state mount. No PostgreSQL resources or Operator are needed.
- **External PostgreSQL:** Provision the database and a non-superuser role.
  Set `database.driver: postgres` and `database.postgres.existingSecret` to a
  same-namespace Secret with a full TCP DSN using `sslmode=verify-full`
  (`database.postgres.dsnKey`, default `dsn`).
- **Managed PostgreSQL:** Set `database.driver: postgres` and leave
  `database.postgres.existingSecret` empty. Install the CloudNativePG Operator
  and CRD separately. The Chart provisions one DB instance on a separate PVC
  with verified TLS. The bootstrap Job uses a stable marker ConfigMap and
  credential Secret. Back up both with the DB. If the Secret is missing but the
  marker remains, the Job fails closed rather than generating a password.

See the [Chart database guide](../chart/README.md#database) for TLS, managed
bootstrap and recovery, and Helm and Argo CD ordering.

Read-only SSH Secrets and projected volumes remain usable for connections, but
SSH config edits and key operations require a writable source. State and
workspace must always be writable by UID/GID 1000.

The Chart mounts a separate `emptyDir` at
`/var/cache/roaminal/filesystem-image-previews`. Its default `192Mi` quota is
the sum of the `128Mi` cache target, one conversion's `32Mi` source and
`16Mi` output staging allowance, and `16Mi` overhead. If concurrency or byte
limits are increased, set `filesystemImagePreview.cache.emptyDir.sizeLimit`
to at least `cache target + maxConversions * (maxSource + maxOutput) + 16Mi`.
Exhausting this volume disables only derivative generation; original views and
downloads continue to use the FileSystem transport.

Graceful shutdown gives terminal and SSH cleanup a separate 10-second
budget, including time to stop the terminal worker. A connection instance
that cannot be archived keeps its active record and last committed checkpoint
for retirement on the next startup. Its live PTY is not resumed. Roaminal
waits for in-flight terminal database calls before releasing the state lock;
storage that does not respond to cancellation can delay process exit.

## Offline maintenance

1. Stop the serving runtime. With the database-enabled Chart, set
   `maintenance.enabled: true` in the desired Helm or GitOps values and
   reconcile. This scales only Roaminal to zero; the PVC and DB resources
   remain. For an older Chart without this flag, pause reconciliation and
   scale the old Deployment to zero. Wait for the Pod to exit and prevent
   other controllers or operators from starting a writer. On a single host,
   stop the container or service and prevent automatic restart.
2. Use the database-enabled runtime image you intend to serve, as UID/GID
   1000 in the container, with `HOME=/home/roaminal`. In Kubernetes, use a
   one-off maintenance Pod in the release namespace, with no serving process
   or Service. Use the same PVC and `persistence.paths.state` subpath at
   `/home/roaminal/.roaminal`, not a copy of the target state mount. Preserve
   the runtime's `/workspace` and `/home/roaminal/.ssh` mappings, including
   separate SSH mounts where used. Prepare the subpaths before mounting them:
   the stopped runtime's state directory and SQLite's `db/` directory must
   belong to the Roaminal UID and be private (`0700`). If a restore added group
   permissions on the **writable destination**, use `roaminal db prepare-state
   --state-dir /home/roaminal/.roaminal` before maintenance. Add
   `--create-sqlite-dir` for the Chart's default SQLite path. When preparing a
   file-era destination for its first database startup, add
   `--repair-legacy-files` to tighten the recognized old records too. The
   command restores modes on known owned paths, not ownership. Keep the
   original backup untouched and preserve modes on the read-only import copy.
3. Use the target (or source for export) Deployment's `ROAMINAL_DB_*` env,
   including the same DSN `secretKeyRef` and read-only CA mount for PostgreSQL
   when configured. Wait for the database and required Secrets to be available.
   Mount legacy copies read-only at a separate path, and exports on private
   backup storage. With the default image entrypoint, pass the arguments
   after `roaminal` (starting with `db` or `admin`) as the container args.
   Run one command at a time; do not start a serving process. For local
   containers, see the [Podman maintenance example](podman/usage.md#offline-maintenance).

## Cut over from legacy files

1. Stop the old runtime before upgrading. Back up the whole legacy
   `$HOME/.roaminal`, workspace, and native SSH sources (including Includes).
   Mount a coherent, read-only copy of `.roaminal` at `/mnt/legacy/state` and
   its SSH copy at `/mnt/legacy/ssh`. Reuse the unified serving claim or
   [prepare one](#migrating-from-an-older-multi-volume-deployment). With the
   old Pod gone and backups in place, stage the database-enabled Chart and
   target DB settings with `maintenance.enabled: true`. Keep the existing
   login password and any Web Push VAPID credentials unchanged.
2. Import into a new or empty database that has never served. Run all three
   modes against the same copy, in order. `--dry-run` inspects the copy only;
   `--apply` writes and verifies the records; `--verify` compares the records
   and recorded source fingerprint before startup.

   ```sh
   roaminal db import-files --source /mnt/legacy/state --dry-run
   roaminal db import-files --source /mnt/legacy/state --apply
   roaminal db import-files --source /mnt/legacy/state --verify
   ```

   `--source` is the copy of the whole `.roaminal` directory, not its possible
   inner `state/` directory. The importer detects the populated root and reads
   top-level sidecars. If needed, `--legacy-root` must select the copied root
   or its `state/` child; it cannot resolve two populated roots. For a prepared
   `definition-mutation.json` journal, add `--ssh-config /mnt/legacy/ssh/config`
   to **each** command. Use a regular file matching the journal's original
   config bytes, or a missing path if the journal records no original file.
   Reconcile any mismatch on a working copy before mounting it read-only;
   the importer does not rewrite SSH config. If the journal remains on the
   serving state mount, the SSH config at startup must also match.
3. Compare the reported counts and fingerprint, and stop if any step fails.
   Verification checks layout revisions and order, message cursors, TOTP
   replay state, and snapshot integrity. If an interrupted retirement left
   both active and audit copies, the importer preserves both when metadata
   and any two snapshots match byte-for-byte. One snapshot may be absent;
   conflicting copies are rejected. Login sessions without a matching
   enrollment are not imported. An identical apply can be retried before
   first startup; a changed source or a destination that has already served
   is rejected. Startup refuses recognizable legacy state without a matching
   completed import. Do not delete legacy files to bypass this guard.
4. After verification, remove the maintenance Pod and disable maintenance (or
   restart the local service) with the verified DB settings. Before reopening
   traffic, check `/healthz` (worker), `/readyz` (worker and database), login
   with the migrated TOTP enrollment if present, and local and SSH
   connections. In rehearsal, also check retained layouts, Agent messages,
   push registration, interrupted uploads, and [TOTP reset](security.md#resetting-totp-enrollment)
   on disposable data. Old connection instances do not resume.

Before accepting new writes, rollback means stopping the new runtime and
restoring the coherent pre-cutover backup with the old image. Once the database
has new writes, a Helm rollback does not copy them back to files. Restore an
agreed backup only with explicit acceptance of that data loss. For databases
already in use, use the [offline database transfer](backup-recovery.md#switching-databases)
instead of changing the driver alone.

## Migrating from an older multi-volume deployment

After the old Pod exits, back up all three source PVCs. Prepare a unified
claim and copy each old volume's *contents*, including dotfiles, from read-only
snapshots into `state/`, `workspace/`, and `ssh/` respectively. For example,
use `cp -a <source>/. <destination>/` in a temporary copier with only the new
claim mounted writable. Verify the file inventory, ownership and private
permissions, and ensure UID/GID 1000 can use the new claim. Set
`persistence.existingClaim` to it and keep `maintenance.enabled: true` through
the file import and verification. Helm does not copy or adopt the old claims.
Keep them intact; remove them manually only after backup and acceptance.

## Network and validation

The cutover describes the current implementation, not a live verification of
Helm, Argo CD, or a production import. Rehearse on disposable data and storage
with the exact image and Chart before scheduling a production cutover.

Run `helm template` with the selected OCI Chart and apply a server-side dry run
before production changes. The Service is ClusterIP; expose it through a TLS
reverse proxy that preserves WebSocket upgrades and uses read/send timeouts of
at least one hour. Do not expose the worker or runtime sockets. For in-cluster
tests use the Helm Service URL directly, never a port-forward.
