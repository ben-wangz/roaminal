# Tests

This directory contains the authoritative human- and AI-readable regression
specifications. Browser cases are executed by an AI agent with Playwright when
needed; there is no second checked-in executable E2E suite to keep in sync.

All cases target a Kubernetes system deployed from Roaminal's `chart/` Helm
Chart. They do not define acceptance coverage for `go run`, a frontend dev
server, Podman, an ad-hoc Pod, or historical Kubernetes resources. Before
testing, the agent must verify the release is fully deployed and ready. If it is
not, follow the **Kubernetes with Helm** procedure in
[the deployment guide](../docs/deployment.md#kubernetes-with-helm) before
continuing.

Start with [the Playwright execution contract](playwright/README.md). Every
feature case is stored in its own Markdown file under a functional subdirectory.
SSH and tmux cases use the disposable [codespace fixture](playwright/fixtures/ssh-codespace.md),
which must be reset before testing and uninstalled after the suite.
