# Repository operating rules

- `container/VERSION` is the single Roaminal product version. Never edit it
  manually; use `setup/forgekit.sh` and `forgekit version bump`.
- Run the root ForgeKit lint gate after changes to source, build, or tooling
  files: `FORGEKIT_BIN="$(bash ./setup/forgekit.sh)"` followed by
  `"$FORGEKIT_BIN" --project-root "$(pwd)" lint --config lint.yaml`.
- `frontend/package.json` and `terminal-worker/package.json` are private module
  manifests. Their `0.0.0` versions are intentionally not release versions and
  must not be bumped with a Roaminal release.
- Keep `deploy/kubernetes/` as ordinary checked-in YAML. Do not convert it to a
  chart as part of routine feature work.
