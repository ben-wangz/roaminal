# Kubernetes deployment migration

The supported Kubernetes source is now [`chart/`](../../chart/). The former raw
manifests are intentionally retired so that deployment behavior is defined in
one place.

For an existing raw-manifest installation, follow the migration procedure in
[`docs/deployment.md`](../../docs/deployment.md) before installing the chart.
The procedure explicitly backs up the three legacy PVCs, copies them into the
single chart PVC under `state/`, `workspace/`, and `ssh/`, and leaves the legacy
PVCs untouched until the operator verifies the result.

Do not apply files from this directory as a deployment; this README is kept as
the discoverable migration entry point only.
