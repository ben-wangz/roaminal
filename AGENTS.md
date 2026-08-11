# Repository operating rules

- `container/VERSION` is the single Roaminal product version. Never edit it
  manually; use `setup/forgekit.sh` and `forgekit version bump`.
- `version-control.yaml` registers only the `roaminal` Chart. The
  `roaminal-runtime` container is a Chart-linked target from `Chart.yaml`, not
  a second top-level app. Read linked runtime metadata from
  `forgekit version get roaminal --output json`; direct `version get
  roaminal-runtime` is not supported by ForgeKit 0.6.1.
- Release tags use `roaminal-v<chart-semver>`. Do not create or move release
  tags before the remote `lint` workflow succeeds.
- GitHub Actions `lint` is the required quality gate for pull requests, `main`,
  and release commits. Local full ForgeKit lint is optional; run focused checks
  for the changed module when useful. Local container builds and Chart packages
  are diagnostic-only and must not publish release artifacts.
- `frontend/package.json` and `terminal-worker/package.json` are private module
  manifests. Their `0.0.0` versions are intentionally not release versions and
  must not be bumped with a Roaminal release.
- Keep `deploy/kubernetes/` as ordinary checked-in YAML. Do not convert it to a
  chart as part of routine feature work.
