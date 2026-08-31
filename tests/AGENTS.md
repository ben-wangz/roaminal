# Playwright test-agent rules

These Markdown cases are executable specifications for AI agents. Read
`playwright/README.md` and the selected case completely before running it or
translating it into Playwright code.

- Every case targets Roaminal installed from the repository Helm Chart in
  `chart/`. Before opening a browser, complete the mandatory Helm deployment
  gate in `playwright/README.md`.
- If the designated release is absent, not `deployed`, or not ready, deploy or
  repair it by following the **Kubernetes with Helm** section of
  [`docs/deployment.md`](../docs/deployment.md#kubernetes-with-helm). Do not
  substitute `go run`, a Vite server, Podman, another deployment source, or an
  ad-hoc Pod. If deployment authority or required inputs are unavailable,
  report the run as `BLOCKED`, not passed or skipped.
- Register browser diagnostics before the first navigation. A test is not
  allowed to pass from visible UI assertions alone.
- Treat every uncaught `pageerror`, `console.error`, and `console.warning` as a
  failure unless the case documents an exact, narrowly scoped expectation.
- Record failed requests, unexpected HTTP `4xx`/`5xx` responses, WebSocket
  creation/close events, and the final URL. Inspect them before declaring the
  case passed.
- Do not use a broad allowlist, ignore all WebSocket errors, or discard output
  because the primary UI action appeared to work. Any exception needs an issue
  reference, an exact matcher, and an expiry condition in the test report.
- Redact passwords, access tokens, refresh tokens, private keys, key-generation
  input, and terminal contents that may contain secrets from traces,
  screenshots, videos, logs, and reports.
- Use the Helm-created Service URL directly. Do not create a port-forward.
- Stateful and destructive cases require a dedicated Roaminal test release or
  an explicitly reset test dataset. Never modify unrelated Pods, SSH hosts,
  Secrets, PVCs, or login sessions.
- Always execute cleanup, even after a failed assertion. Report cleanup
  failures separately because they can contaminate later cases.
