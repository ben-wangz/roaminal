# Tests

This directory contains human- and AI-readable regression specifications.
The current Playwright implementation remains in `frontend/e2e/`; the cases in
`playwright/` define the intended complete browser coverage for future
automation and manual agent-driven validation.

All cases target a Kubernetes system deployed from Roaminal's `chart/` Helm
Chart. They do not define acceptance coverage for `go run`, a frontend dev
server, Podman, an ad-hoc Pod, or historical raw Kubernetes manifests. Before
testing, the agent must verify the release is fully deployed and ready. If it is
not, follow the **Kubernetes with Helm** procedure in
[the deployment guide](../docs/deployment.md#kubernetes-with-helm) before
continuing.

Start with [the Playwright execution contract](playwright/README.md). Every
feature case is stored in its own Markdown file under a functional subdirectory.
