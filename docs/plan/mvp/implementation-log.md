# Roaminal MVP Implementation Log

This log records implementation evidence in the order the approved MVP plan is
executed. A test is only marked passed after its command has run in this
workspace.

## Phase 0 - Baseline

- Date: 2026-08-06 UTC
- Starting repository commit: `2743bb5`
- Starting worktree: clean; branch `main` tracked `origin/main`
- Reference: `/root/temp/Tabminal`, tag `v3.0.40`, commit
  `fbd26d3aff033fd850a6696eccb107520780fd8b`; reference worktree clean
- Reference source was used only as a behavior oracle. No source or runtime
  asset was copied.

### Environment preflight

| Check | Result |
| --- | --- |
| `uname -m` | `x86_64` |
| shell | Bash 5.2.21 |
| Go/compiler | Go 1.26.5, GCC 13.3.0 |
| race compile | `CGO_ENABLED=1 go test -race -count=1 -run '^$' errors` passed |
| Node/npm | Node.js 24.13.1, npm 11.8.0 |
| Podman | 4.9.3, rootful `crun` runtime |
| Kubernetes | kubectl 1.35.4, server v1.34.6+k3s1 |
| Kubernetes context | `develop`, default namespace `develop` |
| Kubernetes access | namespaced resources and port-forward are allowed |
| Chrome | Google Chrome 151.0.7922.75 |
| Registry | HTTPS `/v2/` health returned `{}` |
| Port 9846 | free at preflight |
| container storage | 61G total, 22G available at `/var/lib/containers` |

### Baseline artifacts

- API and WebSocket behavior fixtures: `testdata/protocol/`
- terminal worker framing and snapshot fixtures: `testdata/worker/`
- reference viewport capture notes: `testdata/reference-viewports/`
- third-party license inventory: `THIRD_PARTY_NOTICES.md`

The fixtures are small, reviewable contracts derived from the approved modules
and reference behavior. They deliberately contain no reference source code,
credentials, terminal payloads from a user environment, or copied assets.

## Phase 0 gate

Passed on 2026-08-06: pinned reference, starting worktree, environment baseline,
behavior fixtures, and direct dependency/license inventory are auditable.

## Phase 1 notes

- Frontend fixed dependencies install and build with the repository-local
  `legacy-peer-deps=true` setting because the approved xterm beta addons declare
  an xterm 5 peer while the approved `@xterm/xterm` package is 6 beta.
- `npm run typecheck` and `npm run build` pass. `typescript-eslint@8.66.0`
  refuses to load with the approved TypeScript 7.0.2 (`TS 7.0` is explicitly
  unsupported by that release); `npm run lint` therefore runs ESLint against
  JavaScript configuration files and TypeScript is gated by the project
  typecheck. This is a toolchain limitation, not a dependency version change.
- Phase 1 gate evidence: `go test ./...`, `go vet ./...`, terminal-worker
  `npm test`/syntax check, web `npm run lint`/`typecheck`/`build`, project-local
  Chrome smoke, HMAC login, PTY input, and ordered terminal WebSocket attach all
  passed on the local service.

- Phase 2/3 gate evidence: `go test -race ./...` passes configuration, auth,
  atomic persistence, snapshot checksum/quarantine, worker framing, UTF-8
  chunk handling, HMAC rotation/lockout, and PTY/session integration tests.
  Manual service smoke verified worker handshake, login, session creation,
  ordered attach, Bash input/output, title/cwd metadata, and clean shutdown.

## Phase 4-6 gate evidence

- HTTP routes are an explicit allowlist. Challenge/login/refresh, bearer
  authentication, same-origin checks, heartbeat resize validation, WebSocket
  attach ordering, boot ID detection, and terminal reconnect behavior are
  covered by Go tests and the manual service/container flows.
- React runtime is split into auth, heartbeat, terminal, tabs, preview, search,
  status, touch, and UI modules. The browser request helper preserves bearer
  headers across refresh retries; terminal addons load only after xterm opens its
  viewport. `npm --prefix web run test` reports 3 Vitest tests, and typecheck,
  lint, and build pass.
- `ROAMINAL_E2E_PASSWORD=... npm --prefix web run e2e -- --workers=1` passed
  10/10 tests across 1440x900, 1024x768, 768x1024, 390x844, and 844x390.
  The authenticated test asserted non-empty terminal canvas pixels, no
  horizontal overflow, no Service Worker registration, no external resources,
  and search open/close behavior; viewport screenshots were written to the
  Playwright test output directory.

## Phase 7 gate evidence

- Final image tag:
  `container-registry.internal.pve.lab.geekcity.tech:32443/ben-wangz/roaminal:f103d73f3ccefed86ee058cd5e7fa0184acacf83`.
  `podman build`, `podman push`, and `podman pull` passed. The pulled image
  config ID is `83e08e9e0ab8`, registry manifest digest is
  `sha256:e66aee9c65ef42daf6f40ef7e8cebb0292ec4efd251167d2a52deba9e9f91f3a`,
  and the runtime user is `1000:1000`.
- A read-only-root Podman run with `/tmp` tmpfs and state/workspace volumes
  returned `/healthz` 200 and completed login plus ordered WebSocket attach and
  `CONTAINER_FINAL_OK` PTY output. Runtime inspection found Node but no
  `npm`, `npx`, `corepack`, compiler, or `cloudflared`.
- Kubernetes manifests passed API-server server-side dry-run (the API emitted
  non-fatal last-applied migration warnings). In `develop`, the final image
  reached `1/1` Ready with zero restarts; Service is `ClusterIP`, PVCs are
  bound RWO, and the required probes/resources/security context are active.
  Port-forward API/PTY flow passed, then deleting the only Pod recreated Bash
  while preserving session ID and `K8S_OK` scrollback; the new Bash returned
  `RESTORED_OK`.
- The cluster's local-path driver exposes PVC mount roots as `root:1000` mode
  `2777` and permits UID/GID 1000 to create private children. Commits
  `17e92a4` and `583d637` make persistence accept that fsGroup policy only by
  placing sensitive files below a private `state/` directory; non-writable or
  world-accessible private directories still fail fast. No Pod privilege or
  root init container was added.

## Phase 8 gate evidence

- Final commands passed: `go test ./...`, `go vet ./...`,
  `CGO_ENABLED=1 go test -race ./...`, terminal-worker lint/tests, Vitest,
  frontend typecheck/lint/build, five-viewport Chrome E2E, Podman smoke, and
  Kubernetes rollout/restore smoke.
- Scope scan found no executable Host/cluster namespace, file UI/API, Agent/ACP/
  AI/provider, PWA/Service Worker, CDN, cloudflared, Docker/Compose/Make, or
  native-client artifacts. `README.md` mentions excluded features only to make
  the product boundary auditable.
- Documentation shipped: README, API, configuration, security, deployment,
  backup/recovery, and troubleshooting guides. No secrets or temporary
  Kubernetes manifests are tracked.

### Residual toolchain notes

- `typescript-eslint@8.66.0` does not support the approved TypeScript 7.0.2;
  ESLint intentionally ignores TypeScript sources while `tsc --noEmit` gates
  them. This is recorded as a fixed toolchain limitation.
- Podman warns that OCI output ignores the image `HEALTHCHECK`; Kubernetes and
  the integration harness use the documented `/healthz` probes directly.

## Terminal interaction revision evidence

- Date: 2026-08-07 UTC. The approved addendum requirements and solution are
  implemented by the pushed commits `4ed4855`, `dc5b517`, `e784e9c`, `a6f15cc`,
  `9c8db60`, `2f29c73`, `7d65581`, `7e99a3d`, `84c05d9`, `2bfac9d`, `a6e6228`,
  `d7f103e`, `053bdd3`, and `ca47231`.
- Backend evidence: `go test ./...`, `go vet ./...`, and
  `CGO_ENABLED=1 go test -race ./...` passed. Session summaries sort by
  `createdAt, id`; title PATCH, v1-to-v2 migration, strict title validation,
  automatic-title reset, and direct/private-child/ambiguous state layout tests
  passed. Startup logs report only `state layout=direct` or
  `state layout=private-child`.
- Frontend evidence: `npm --prefix web test` reports 6 Vitest tests; web
  typecheck, lint, and production build pass. `npm ls` for xterm core and all
  browser addons exits cleanly with no `invalid` peer dependency. The old
  CanvasAddon is not installed or loaded.
- Kubernetes image and rollout evidence:
  `container-registry.internal.pve.lab.geekcity.tech:32443/ben-wangz/roaminal:053bdd38a2beecf0caad4074755c06668fa88a1a`
  was built and pushed with Podman, then reached `1/1` Ready in `develop` with
  zero restarts. The pod log reported `Roaminal state layout=private-child`.
  The Service remains `ClusterIP` on port 9846 and the state PVC contains active
  data only below `.roaminal/state/`; the root `sessions/` directory is empty.
- Chrome evidence: `npm --prefix web run e2e -- --workers=1` ran 25 cases;
  13 passed and 12 were intentionally skipped by project filter. The passing
  desktop/phone cases cover 100 Tab switches with exactly one direct xterm,
  Close tab with no DELETE request, title persistence across reload and reset
  to automatic title, menu keyboard navigation/focus restoration, terminate
  cancel/confirm, desktop sidebar geometry/focus, mobile overlay/backdrop and
  Escape, five viewport rendering, no external resources and no page/console
  errors. A Chrome link-hover smoke moved across the xterm screen after URL
  output and observed `xterms=1`, `screens=1`, favicon `/favicon.svg`, and an
  empty error list.

## Single-terminal sidebar remediation

- Date: 2026-08-07 UTC. The approved module 13 implementation is split across
  `24b5b6a`, `74ae8b7`, `4cfd8f1`, `ac42655` and `895a136`.
- Frontend now stores only `activeSessionId`; the top Terminal Tab bar, open-tab set,
  Close Tab command and related CSS/localStorage model were removed. Sidebar cards
  show stable short ID, full PWD tooltip and browser-local SINCE, with a single main
  xterm/runtime. Desktop hover uses one disposable read-only preview runtime; mobile
  layouts do not create previews. Agent and Files are visible Lucide unavailable
  affordances; Terminal actions contain rename, automatic-title reset and terminate.
- Auth now rejects insecure origins with `Secure HTTPS context required`, performs
  server logout before local cleanup, and exposes login-session revoke/logout-others
  controls. Heartbeat supplies latency, configured scrollback and persistence warning;
  execution completion has lifecycle-bounded toast/notification handling. Touch
  modifiers, visualViewport observation and the shortcut registry are connected.
- Go evidence after the remediation changes: `go test -race ./...`, `go vet ./...`;
  frontend evidence: `npm --prefix web run typecheck`, `npm --prefix web test`,
  `npm --prefix web run build`; worker evidence: `npm --prefix terminal-worker test`
  and syntax check. Added tests cover attach reservation atomicity, control-owner
  rejection and `1013 slow_client` reason propagation.
- WebSocket attach now reserves capacity before upgrade and returns HTTP `429`; main
  clients explicitly claim control while preview clients remain read-only. The worker
  uses a 16 MiB mutation writer budget, a 10 second stall deadline and exact
  `xterm-headless 5.3.0`/`xterm-addon-serialize 0.11.0` handshake validation. Restore
  initializes worker state before publishing the Bash session or starting PTY loops;
  concurrent session creation and cwd inheritance are deterministic.
- Kubernetes Chrome configuration is parameterized by `ROAMINAL_E2E_BASE_URL` and
  defaults to `http://roaminal.develop.svc.cluster.local:9846`; no port-forward is
  part of the gate. Final direct-Service evidence is recorded below.

## Single-terminal sidebar final gate

- Date: 2026-08-07 UTC. The final remediation code is present in `62b3c56`,
  `0cfe82e`, `435ad67`, `16570af`, `46000fe` and the attention refinement
  `82b97ac`.
- Persistence now tracks degraded checkpoints by session ID and only clears a
  recovered session; worker snapshot failures mark that session before the next
  checkpoint. Restore and create initialize the worker before publishing Bash or
  starting PTY loops, and rollback closes an already-created worker session.
- Session summaries expose `attention` for completed executions waiting on a
  non-current session; ordinary shell prompt output does not create false attention.
  Claiming terminal control clears it; Sidebar renders a text state plus a color
  independent indicator. Preview creation has a 100ms intent delay and generation
  guard, and Modal dialogs trap focus and close on Escape.
- Automated local gate passed: `go test ./...`, `go test -race ./...`, `go vet ./...`,
  `npm --prefix terminal-worker test`, `npm --prefix terminal-worker run lint`,
  `npm --prefix web test -- --run` (5 files, 15 tests),
  `npm --prefix web run typecheck`, and `npm --prefix web run build`.
- Frontend contract tests cover legacy active-session migration, fixed ID/PWD/SINCE
  formatting, shortcut matching, secure-context fail-closed behavior, action and
  modal paths, 100 active-session switches, one main xterm, preview before/after
  screenshots and pointer-leave disposal. Artifacts are under
  `web/test-results/smoke-sidebar-cards-switch-a61ca--and-expose-preview-actions-chrome-desktop/`.
- Final image:
  `container-registry.internal.pve.lab.geekcity.tech:32443/ben-wangz/roaminal:82b97ac4930df069871cd97a24d11263eca7bd08`.
  Podman config ID is `5057a45209f5e06545a69b02d42a712252ddaeaa9ab8b6f9a96a9ed839729ace`,
  local image digest is
  `sha256:b30738464f0df042393037be0cd5e375a8b49e095bf010bd42f28907cd73647e`,
  and the running Pod reports registry image ID
  `sha256:b19943cf5a728b226b7bfdb1b24bfde2ba3dbdc161b3eba3524eea23ab94d110`.
- Kubernetes server-side dry-run and actual apply completed in namespace `develop`.
  Deployment reached `1/1` Ready with zero restarts; Pod log reports
  `Roaminal state layout=private-child`. Direct
  `curl http://roaminal.develop.svc.cluster.local:9846/healthz` returned
  `{"status":"ok"}`.
- Direct-Service Playwright ran 30 cases with 21 passed and 9 expected skips from
  desktop-only/mobile-only project filters. It covered auth session management and
  server logout, card metadata and extensions, 100 switches, preview cleanup,
  mobile overlay behavior, and no `onShowLinkUnderline` or other page/console error.
  No port-forward was started or required.

### Final residual notes

- The nine Playwright skips are intentional matrix filters, not unrun viewport
  coverage; each of the five configured Chrome viewports ran its applicable cases.
- `typescript-eslint@8.66.0` still does not support the approved TypeScript 7.0.2;
  TypeScript sources are gated by `tsc --noEmit` and JavaScript configuration files
  are linted. Podman also warns that OCI output ignores the image `HEALTHCHECK`; the
  Kubernetes `/healthz` probes are authoritative.
