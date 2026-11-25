# Helm values override and data migration

The supported Kubernetes source is [`chart/`](../../chart/), and published
deployments use the OCI Chart from GHCR. Deployment behavior is defined by that
Chart.

[`values.yaml`](values.yaml) is the repository-level override file for Helm
deployments. It is intentionally empty until this deployment needs to override
a Chart default; an empty file changes nothing.

The Chart runs one serving replica and requires a Roaminal PVC with either
DB driver. SQLite is the default at
`/home/roaminal/.roaminal/db/roaminal.sqlite3` on that PVC. PostgreSQL is opt-in;
see the [Chart database guide](../../chart/README.md#database) for external DSN
Secrets and managed CloudNativePG.

Before upgrading from a file-backed runtime, follow the
[offline cutover](../../docs/deployment.md#cut-over-from-legacy-files). Keep
`maintenance.enabled: true` while you dry-run the read-only legacy copy, then
apply and verify it against the target DB. If moving from three PVCs,
[prepare the unified claim](../../docs/deployment.md#migrating-from-an-older-multi-volume-deployment)
first; the old PVCs remain intact. For installations already using a DB,
[export, import, and activate](../../docs/backup-recovery.md#switching-databases)
before changing the serving database.

Do not apply files from this directory directly to Kubernetes. Pass
`--values deploy/kubernetes/values.yaml` when using the
[published Chart](../../chart/README.md#forgekit-versions). Use a Chart and
image that both support the database settings.
