# Helm values override and data migration

The supported Kubernetes source is [`chart/`](../../chart/), and published
deployments use the OCI Chart from GHCR. Deployment behavior is defined by that
Chart.

[`values.yaml`](values.yaml) is the repository-level override file for Helm
deployments. It is intentionally empty until this deployment needs to override
a Chart default; an empty file changes nothing.

For an existing older multi-volume installation, follow the migration procedure in
[`docs/deployment.md`](../../docs/deployment.md) before installing the chart.
The procedure explicitly backs up the three source PVCs, copies them into the
single Chart PVC under `state/`, `workspace/`, and `ssh/`, and leaves the source
PVCs untouched until the operator verifies the result.

Do not apply files from this directory directly to Kubernetes. Use the values
file with the published Chart, for example:

```sh
helm upgrade --install roaminal \
  oci://ghcr.io/ben-wangz/roaminal-charts/roaminal \
  --version '<chart-version>' \
  --namespace develop --create-namespace \
  --values deploy/kubernetes/values.yaml \
  --set auth.existingSecret=roaminal \
  --set app.acceptTerms=true
```

This README remains the discoverable migration entry point for installations
that still use the older multi-volume layout.
