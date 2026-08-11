# Repository operating rules

- Roaminal is a connection platform. Use `connection definition` and
  `connection instance` for product concepts; reserve `session` for login/auth
  sessions, tmux sessions, or strictly internal PTY details.
- Backend, frontend, and terminal-worker code belongs in `backend/`, `frontend/`,
  and `terminal-worker/`. Container build files belong in `container/`.
- `container/VERSION` is the runtime version. The Helm Chart version is
  independent. Use ForgeKit to mutate versions and synchronize Chart
  `appVersion` and `image.tag`; never edit managed version fields manually.
- `version-control.yaml` registers only the `roaminal` Chart. Read its linked
  `roaminal-runtime` metadata from `forgekit version get roaminal --output json`.
- GitHub Actions `lint` is the required quality gate. Local full lint, container
  builds, and Chart packages are optional diagnostics and must not publish
  release artifacts.
- Release tags use `roaminal-v<chart-semver>` and may be created only after the
  exact release commit passes the remote `lint` workflow.
- `chart/` is the only Kubernetes template source. `deploy/kubernetes/` contains
  only the repository values override and migration guidance; do not add raw
  manifests or another Chart there.
- Frontend and terminal-worker package versions remain private `0.0.0` values.
  They are not Roaminal release versions.
- Browser regression cases are maintained as AI-agent specifications in
  `tests/playwright/`. Do not recreate a second checked-in Playwright suite.
- Keep documentation concise and English-only. Remove completed planning files
  instead of retaining them as permanent product documentation.
