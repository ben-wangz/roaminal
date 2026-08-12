# Roaminal Playwright regression specification

## Purpose

These cases describe end-to-end browser behavior supported by the current
Roaminal code. They are written for AI agents to execute through Playwright.
The Markdown cases are the maintained browser regression specification.

The system under test is exclusively a Kubernetes release installed from this
repository's `chart/` Helm Chart. A locally started backend/frontend, Podman
container, ad-hoc Pod, or retired `deploy/kubernetes/` manifest is not an
acceptable substitute because it does not exercise the Chart's init container,
Secret wiring, unified PVC, probes, Service, security context, or lifecycle.

Passing means both of the following are true:

1. The user-visible behavior and the relevant HTTP/WebSocket state satisfy the
   case assertions.
2. The browser diagnostics gate is clean. A screenshot that looks correct is
   never sufficient by itself.

## Mandatory Helm deployment gate

This gate runs before every test run and before any browser context is created.
The agent must know the operator-designated release and namespace; suggested
variables are `ROAMINAL_E2E_HELM_RELEASE` and
`ROAMINAL_E2E_NAMESPACE`. Defaults may be `roaminal` and `develop`, but an agent
must not guess when the environment names another target.

Verify all of the following:

1. `helm status <release> --namespace <namespace>` succeeds and reports
   `STATUS: deployed`. Record release name, namespace, revision, Chart version,
   app version, and deployment timestamp.
2. Exactly one Roaminal Deployment and Pod belong to that Helm release by
   `app.kubernetes.io/instance=<release>` and
   `app.kubernetes.io/name=roaminal`. The Deployment uses the Chart's `Recreate`
   strategy, its observed generation is current, rollout has completed, and the
   Pod is Running and Ready.
3. The Pod and owned resources carry Helm ownership/Chart labels. The runtime
   uses the intended immutable image, the password comes from the configured
   existing Secret, and no secret value is printed while checking it.
4. The unified PVC is Bound when persistence is enabled. The Pod has the Chart
   mounts for `/home/roaminal/.roaminal`, `/workspace`, and the configured SSH
   source; init-container completion and application startup/readiness/liveness
   probes have no unresolved failure.
5. The Helm-created Service selects that release's ready Pod and has at least
   one ready endpoint. No Service exposes the terminal worker or a storage/runtime
   socket.
6. Through the exact URL assigned to `ROAMINAL_E2E_BASE_URL`, `GET /healthz`
   returns `200 {"status":"ok"}` and `GET /api/version` returns
   `name=roaminal`, the expected API version, product version, and a non-empty
   `bootId`. Access the Service/Ingress directly; never use a port-forward.

If the release is missing or any readiness condition fails, do not start
Playwright. Deploy or repair the system by following the **Kubernetes with
Helm** section of [the deployment guide](../../docs/deployment.md#kubernetes-with-helm)
and the [Chart README](../../chart/README.md). That procedure owns Secret
creation, `helm upgrade --install`, rollout verification, unified PVC setup,
and direct Service access. Do not improvise another deployment path. After the
deployment procedure, rerun this gate from the beginning.

If the agent lacks cluster permission, a required image/Secret/storage input,
or authorization to create the dedicated test release, stop and report
`BLOCKED: Helm deployment prerequisite unavailable`. Environment absence is
not a passed test and must not be hidden as a capability skip.

## Environment contract

Use the Helm release that passed the deployment gate and access its
Helm-created Service directly. Stateful or destructive cases require a
dedicated test release. Do not use a port-forward. Supply secrets through
environment variables, never command-line arguments that will be copied into
reports.

Browser authentication requires a secure context because it uses Web Crypto.
Production-style runs use HTTPS. When the direct test Service is HTTP, launch
Chrome with the repository's existing narrowly scoped
`--unsafely-treat-insecure-origin-as-secure=<base-url>` setting; do not disable
web security globally.

| Capability | Suggested variable | Requirement |
| --- | --- | --- |
| Core | `ROAMINAL_E2E_BASE_URL` | Direct `http://` or `https://` Service/Ingress URL |
| Core | `ROAMINAL_E2E_PASSWORD` | Password for the dedicated test release |
| Mutable SSH | `ROAMINAL_E2E_SSH_ALIAS` | Unique alias owned by the suite |
| SSH transport | `ROAMINAL_E2E_SSH_HOST`, `ROAMINAL_E2E_SSH_USER`, `ROAMINAL_E2E_SSH_PORT` | Dedicated reachable OpenSSH fixture |
| Tmux | `ROAMINAL_E2E_TMUX_ALIAS` | SSH fixture with `tmux` installed and a writable home |
| Restart | test release and namespace identifiers | Permission to restart only the dedicated Roaminal workload |

The SSH fixture must accept an Ed25519 key owned by the test release, must not
contain production data, and must be safe for shell exit, tmux session creation,
and repeated connections. Tests that mutate `~/.ssh/config`,
`~/.roaminal/ssh-connection-options.yaml`, SSH keys, auth sessions, or the
connection count run serially unless every worker has its own release and PVC.

Before a run, retain the Helm deployment-gate record together with the Roaminal
version and `bootId`, browser/project name, viewport, and enabled capabilities.
A missing optional test fixture capability produces an explicit
`SKIPPED: <reason>` result; a missing/unready Helm deployment is `BLOCKED` and
must be deployed or repaired before testing.

## Standard projects

Run core visual and interaction coverage in the five standard viewports defined
by this contract:

| Project | Viewport |
| --- | --- |
| desktop | 1440 x 900 |
| tablet landscape | 1024 x 768 |
| tablet portrait | 768 x 1024 |
| phone portrait | 390 x 844 |
| phone landscape | 844 x 390 |

Run long-lived, multi-browser, restart, key-management, SSH-config mutation,
and tmux cases once on desktop unless their case says otherwise. Responsive
coverage has its own case.

## Mandatory browser diagnostics gate

Install listeners before the first `page.goto()` and retain their output until
all assertions and cleanup observations finish:

- `page.on('console')`: capture type, text, source URL, and location. Fail on
  `error` and `warning` by default.
- `page.on('pageerror')`: fail on every uncaught exception.
- `page.on('requestfailed')`: capture URL, method, resource type, and failure.
  Fail unless the case caused and documented that exact cancellation.
- `page.on('response')`: fail unexpected `4xx` and all `5xx`; an expected
  negative response must match method, path, status, and error body.
- `page.on('websocket')`: record URL, frames, socket errors, and close. Verify
  that every connection uses the expected same-origin `/ws/connection-*` path.

Before marking a case passed, explicitly assert that the collected diagnostics
contain none of the following known regressions:

- `onShowLinkUnderline`
- `terminal runtime ... is disposed`
- `WebSocket is closed before the connection is established`
- `HTTP Authentication failed; no valid credentials available`
- unexpected `invalid session id`
- unexpected `ssh transport unavailable`
- unexpected `ssh transport is draining`

Warnings are evidence, not noise. If a browser or dependency adds a benign
warning, capture it in the report and fix the product or add a narrow temporary
exception tied to a tracked issue. Never add `message.includes(...)` exclusions
without explaining the originating request and expiry condition.

## Common execution rules

1. Confirm the mandatory Helm deployment gate passed for this run. Then create
   a fresh browser context per case, attach diagnostics, and navigate.
2. Authenticate through the visible login UI unless a case is specifically
   testing an already authenticated storage state.
3. Prefer accessible roles, labels, and stable `data-*` identifiers. Do not
   locate hashed bundle names or depend on generated CSS ordering.
4. Use polling assertions for PTY output, heartbeat publication, monitor
   sampling, and WebSocket reconnection. Do not replace readiness checks with
   fixed sleeps.
5. Correlate mutations with their API request and response. For terminal input,
   assert the resulting terminal output or remote state, not only a button click.
6. Take screenshots at the meaningful final state and on failure. Traces and
   videos must be retained on failure and redacted when necessary.
7. Restore definitions, keys, tmux sessions, auth contexts, and connection
   instances created by the case. Never delete pre-existing data.
8. At the end, run the diagnostics gate, confirm cleanup, and write a result with
   `PASS`, `FAIL`, or `SKIPPED` plus evidence paths.

## Coverage index

| Area | Cases |
| --- | --- |
| Authentication | [login](auth/01-login.md), [token refresh](auth/02-token-refresh.md), [login sessions](auth/03-login-sessions.md), [sign out](auth/04-sign-out.md) |
| Connection definitions | [manager/filter](connections/01-manager-and-filter.md), [source capabilities](connections/02-ssh-config-source.md), [create/edit](connections/03-definition-create-edit.md), [copy/delete/ETag](connections/04-definition-copy-delete-etag.md) |
| Connection lifecycle | [local](connections/05-local-connection.md), [SSH](connections/06-ssh-initial-connect.md), [reuse](connections/07-transport-reuse.md), [tmux](connections/08-tmux.md), [pending launch](connections/09-pending-launch.md), [exit/failover](connections/10-exit-and-failover.md), [source change](connections/11-source-change-draining.md) |
| SSH keys | [inventory/copy](keys/01-inventory-and-copy.md), [generation](keys/02-generation.md), [delete/read-only](keys/03-delete-and-readonly.md) |
| Workspace | [terminal I/O](workspace/01-terminal-io.md), [sidebar/preview](workspace/02-sidebar-selection-and-preview.md), [actions/titles](workspace/03-actions-and-titles.md), [display names](workspace/04-display-name.md), [search](workspace/05-search.md), [context keyboard](workspace/06-contextual-keyboard.md), [touch keyboard](workspace/07-touch-keyboard.md), [resize](workspace/08-resize.md), [local status](workspace/09-system-status.md), [remote monitor](workspace/10-remote-monitor.md), [responsive/a11y](workspace/11-responsive-and-accessibility.md), [terminal appearance](workspace/12-terminal-appearance.md) |
| Reliability | [refresh recovery](reliability/01-refresh-recovery.md), [WebSocket reconnect](reliability/02-websocket-reconnect.md), [backend restart](reliability/03-backend-restart-persistence.md), [multi-browser control](reliability/04-multi-browser-control.md), [capacity/isolation](reliability/05-capacity-and-isolation.md), [client diagnostics](reliability/06-client-diagnostics.md) |
| Security | [browser boundary](security/01-browser-security.md), [HTTP/WebSocket authorization](security/02-http-websocket-authorization.md), [HTTPS ingress WebSocket](security/03-https-ingress-websocket.md) |
