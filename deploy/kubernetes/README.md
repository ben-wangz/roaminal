# Helm values and legacy migration

The supported Kubernetes source is now [`chart/`](../../chart/), and published
deployments use the OCI Chart from GHCR. The former raw manifests are
intentionally retired so that deployment behavior is defined in one place.

[`values.yaml`](values.yaml) is the repository-level override file for Helm
deployments. It is intentionally empty until this deployment needs to override
a Chart default; an empty file changes nothing.

For an existing raw-manifest installation, follow the migration procedure in
[`docs/deployment.md`](../../docs/deployment.md) before installing the chart.
The procedure explicitly backs up the three legacy PVCs, copies them into the
single chart PVC under `state/`, `workspace/`, and `ssh/`, and leaves the legacy
PVCs untouched until the operator verifies the result.

Do not apply files from this directory as Kubernetes manifests. Use the values
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
that still use the retired raw manifests.
