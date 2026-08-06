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
