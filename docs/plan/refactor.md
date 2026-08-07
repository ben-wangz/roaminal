# Roaminal repository refactor and ForgeKit adoption plan

Status: proposed. This document is an execution contract for an implementation
agent. Do not start implementation until the user explicitly asks to execute
this plan.

## 1. Outcome

Refactor Roaminal into three explicit source modules and introduce ForgeKit as
the repository entry point for linting and product version management:

- all Go backend source and backend runtime resources live under `backend/`;
- all browser frontend source and frontend-owned tests/assets live under
  `frontend/`;
- `terminal-worker/` keeps its current top-level directory name and remains an
  independent Node module;
- `container/Containerfile` is the only production assembly boundary between
  the three modules;
- ForgeKit `v0.6.1` owns the single Roaminal product version in
  `container/VERSION`;
- root `lint.yaml` defines the complete repeatable repository lint gate;
- GitHub Actions runs the same ForgeKit lint command used locally;
- normal shell `exit` leaves the application usable and preserves the closed
  terminal as read-only history;
- the top bar reports cgroup-scoped CPU and memory rather than host resources.
- `tini` remains PID 1 and the complete signal/reaping chain is verified for
  service shutdown and terminal process groups.

Except for the explicitly specified compatible monitoring fields and terminal
exit fix, the refactor must preserve the external HTTP/WebSocket API, persisted
state format, Kubernetes Service identity, authentication behavior, terminal
behavior, and browser UI behavior.

## 2. Instructions for the implementation agent

1. Read this entire document before changing files.
2. Inspect `git status` first. Preserve user changes. The known MVP-plan cleanup
   and this plan may still be uncommitted; do not discard or overwrite them.
3. Capture the baseline test results before moving code. A failing baseline is a
   stop condition only when the failure prevents proving behavioral equivalence.
4. Use `git mv` for tracked moves so history remains understandable.
5. Execute the phases in order. Do not combine unrelated cleanup or feature work
   with this refactor.
6. Create the atomic commits listed in section 12 when implementation is
   authorized. Stage files explicitly so pre-existing changes are not mixed by
   accident.
7. Do not use port forwarding. Kubernetes and Playwright verification must use
   the direct Service URL.
8. Do not ask for confirmation between phases unless a stop condition in section
   14 is met.

## 3. Reference findings

The ForgeKit design is based on `/root/code/github/k8s-at-home` at the state
reviewed on 2026-08-07 and on the local ForgeKit `v0.6.1` source.

Relevant reference behavior:

- `setup/forgekit.sh` pins both a minimum and preferred ForgeKit version,
  installs to the ignored repo-local `build/bin/forgekit`, verifies checksums,
  and works independently of the caller's current directory;
- `version-control.yaml` is the canonical registry for chart, container, and
  binary targets;
- a container target reads SemVer from its target directory's `VERSION` file;
- `forgekit version get`, `version get --git`, and `version bump` provide the
  read and mutation flows;
- `lint.yaml` combines file-size policy with named commands;
- CI bootstraps the pinned ForgeKit binary and invokes the same root lint config;
- ForgeKit publishing can consume the container target later, but registry
  publication automation is intentionally not part of this refactor.

Do not copy application-specific chart or release workflow logic from
`k8s-at-home`. Roaminal is one deployable product, not a registry of separately
released applications.

The top status design also references `/root/temp/Tabminal`, specifically
`src/system-monitor.mjs`, `public/app.js:updateSystemStatus`, and the
`system-status-bar` styles. Tabminal displays host, kernel, IP, CPU, memory,
uptime, session count, FPS, and heartbeat data, but its Node `os.cpus()`,
`os.totalmem()`, and `os.freemem()` approach reports host-level capacity inside
the Roaminal container and must not be copied.

## 4. Fixed architectural decisions

### 4.1 One product version

Roaminal has one release version because the shipped artifact always contains
the backend, frontend, and terminal worker together. Do not create independent
ForgeKit versions for those implementation modules.

The root registry must be:

```yaml
apps:
  - name: roaminal
    type: container
    path: container
    versionFile: VERSION
```

Initialize `container/VERSION` to `0.1.0`, matching the current hard-coded
backend version. After introduction, never edit `container/VERSION` manually;
use:

```sh
FORGEKIT_BIN="$(bash ./setup/forgekit.sh)"
"$FORGEKIT_BIN" --project-root "$(pwd)" version get roaminal
"$FORGEKIT_BIN" --project-root "$(pwd)" version get roaminal --git
"$FORGEKIT_BIN" --project-root "$(pwd)" version bump container roaminal patch
```

Release tags use `roaminal-v<semver>`. Private npm package versions are not
release versions; normalize both private package manifests and lockfile roots to
`0.0.0` and document that they are not bumped during a Roaminal release.

### 4.2 Module boundaries

The dependency direction is fixed:

```text
frontend build output -----\
                            +--> container/Containerfile --> Roaminal image
backend binary ------------/
terminal-worker runtime ---/

browser <-- HTTP/WebSocket --> backend <-- framed worker protocol --> terminal-worker
```

- Backend must not import frontend source or generated assets as a Go package.
- Frontend must not import backend source. Its only dependency is the documented
  HTTP/WebSocket contract.
- Terminal worker must not import backend source. Its only dependency is the
  framed worker protocol and fixtures.
- `container/Containerfile` may reference all modules because it is integration
  infrastructure, not an application module.

### 4.3 Frontend serving

Remove the Go `embed` copy under `internal/webassets/dist`. The frontend build
must produce only `frontend/dist`, which remains ignored by Git.

Backend must serve a configured static directory through a backend-owned HTTP
adapter. Add the following configuration without changing existing precedence:

| Field | CLI | Environment | Development default |
| --- | --- | --- | --- |
| `frontendDir` | `--frontend-dir` | `ROAMINAL_FRONTEND_DIR` | `../frontend/dist` |

Requirements for the adapter:

- resolve and validate the directory during startup;
- require a readable regular `index.html` and fail startup with a precise error
  if production static assets are missing;
- preserve current `Cache-Control` behavior for `index.html`, hashed `assets/`,
  and other static files;
- use structured filesystem APIs (`os.DirFS`, `fs.ValidPath`, and
  `http.FileServer`) rather than path concatenation;
- preserve API and WebSocket routing precedence;
- keep traversal attempts outside the configured directory inaccessible;
- accept a handler or filesystem dependency in server tests so tests do not
  depend on a checked-in frontend build.

The runtime image sets `ROAMINAL_FRONTEND_DIR=/opt/roaminal/frontend` and copies
the frontend builder's output there.

### 4.4 Go module identity

Move the Go module to `backend/go.mod` and change its module path to:

```text
github.com/ben-wangz/roaminal/backend
```

Update all internal imports accordingly. Do not add a root `go.mod` or `go.work`:
there is only one Go module after the frontend asset shim is removed. All Go
commands must use `go -C backend ...`.

## 5. Target directory layout

```text
.
|-- AGENTS.md
|-- .containerignore
|-- version-control.yaml
|-- lint.yaml
|-- backend/
|   |-- cmd/roaminal/main.go
|   |-- go.mod
|   |-- go.sum
|   |-- internal/
|   |   |-- auth/
|   |   |-- buildinfo/
|   |   |-- config/
|   |   |-- frontend/
|   |   |-- monitor/
|   |   |-- persistence/
|   |   |-- server/
|   |   |-- terminal/
|   |   `-- worker/
|   `-- shell/roaminal-bashrc
|-- frontend/
|   |-- e2e/
|   |-- public/
|   |-- src/
|   |-- testdata/reference-viewports/
|   |-- package.json
|   |-- package-lock.json
|   `-- tool configuration files
|-- terminal-worker/
|   |-- src/
|   |-- test/
|   |-- testdata/
|   |-- package.json
|   `-- package-lock.json
|-- container/
|   |-- Containerfile
|   `-- VERSION
|-- testdata/protocol/
|-- deploy/kubernetes/
|-- docs/
|-- setup/forgekit.sh
`-- .github/workflows/lint.yaml
```

`testdata/protocol/` remains at root because it is a cross-module external
contract fixture. All other fixtures move to the module that owns them.

## 6. Exact path migration

| Current path | Target path | Notes |
| --- | --- | --- |
| `cmd/` | `backend/cmd/` | Update build and run commands. |
| `internal/` | `backend/internal/` | Split files as required by section 8. |
| `internal/webassets/` | removed | Replace with `backend/internal/frontend/` static adapter; do not retain embedded dist. |
| `shell/` | `backend/shell/` | Update local fallback and image copy paths. |
| `go.mod`, `go.sum` | `backend/go.mod`, `backend/go.sum` | Change module path to `/backend`. |
| `web/` | `frontend/` | Preserve npm history with `git mv`. |
| `frontend/go.mod` after move | removed | The old web-assets shim is no longer needed. |
| `testdata/reference-viewports/` | `frontend/testdata/reference-viewports/` | Frontend-owned visual fixture documentation. |
| `testdata/worker/` | `terminal-worker/testdata/` | Update worker test import to a module-local path. |
| `testdata/protocol/` | unchanged | Shared external contract fixture. |
| `terminal-worker/` | unchanged | Only internal fixture paths and package metadata may change. |
| `Containerfile` | `container/Containerfile` | Preserve root build context; move the build definition only. |
| new product version | `container/VERSION` | The only ForgeKit-managed release version. |

After the move, these top-level paths must not exist: `cmd/`, `internal/`,
`shell/`, and `web/`.

## 7. Backend refactor details

### 7.1 Build information

Add `backend/internal/buildinfo/buildinfo.go` with a development default such as
`Version = "dev"`. Remove the hard-coded `var version = "0.1.0"` from `main`.
Both `GET /api/version` and a new read-only `roaminal --version` command must use
`buildinfo.Version`.

The container build injects the ForgeKit-resolved version with Go `-ldflags -X`.
The exact symbol is:

```text
github.com/ben-wangz/roaminal/backend/internal/buildinfo.Version
```

### 7.2 Runtime path resolution

Update local fallback paths for the new working directory:

- frontend: `../frontend/dist`;
- terminal worker: `../terminal-worker/src/index.mjs`;
- shell rc: `shell/roaminal-bashrc` when running with `go -C backend`, plus the
  existing absolute production path.

Explicit configuration and environment paths continue to take precedence over
fallbacks. Add focused tests that change the current working directory and prove
the fallback resolution, rather than relying only on the normal developer cwd.

### 7.3 Behavior preservation

Apart from the explicit normal-exit lifecycle fix and backward-compatible
monitoring additions below, do not change:

- API paths, existing JSON fields, auth cookies, token lifetimes, or same-origin
  checks;
- worker protocol/version handshake;
- state directory layout or session format versions;
- unrelated PTY lifecycle, ownership, backpressure, attention state, or restore
  behavior;
- public port `9846` or health semantics.

### 7.4 Terminal `exit` lifecycle bug

Treat this as a required bug fix in the same implementation, not as an assumed
one-line frontend issue. The reported reproduction is:

1. open or select a live terminal;
2. execute the shell builtin `exit`;
3. the shell process exits;
4. Roaminal enters a state in which the user can no longer operate normally.

The implementation agent must reproduce the bug with browser and backend tests
before changing behavior, then trace the complete lifecycle rather than relying
on this plan for a guessed root cause. Audit at least:

- `Session.waitLoop`, `readLoop`, snapshot scheduling, worker session cleanup,
  client broadcast/close ordering, control ownership, and persistence writes;
- WebSocket close codes and whether the frontend treats normal process exit as a
  reconnectable transport failure;
- heartbeat reconciliation of `closed` and `exitStatus`;
- active-session selection when the active session is closed;
- input/resize/search/preview effects that continue after termination;
- deletion of a closed session and creation of the next session;
- the last-live-session case and restore behavior after a service restart.

Required behavior after the fix:

- a normal `exit` transitions exactly that session to an authoritative `closed`
  state and records the exit code/signal once;
- final PTY output and the final worker snapshot remain readable as history;
- the frontend stops input, resize, ownership, and reconnect attempts for the
  closed session without entering an error loop;
- a closed active session renders an explicit read-only exited state with its
  exit status and working actions to create a new terminal and delete/close the
  historical session;
- all global navigation, sidebar actions, auth actions, and other live terminals
  remain operable;
- if another live session exists, the user can switch to it and type
  immediately; if none exists, the create-terminal action produces a usable
  shell without a page reload;
- normal exit is not logged or surfaced as a worker fatal error and does not
  degrade persistence for unrelated sessions;
- deleting a closed session releases its backend and worker resources
  idempotently.

Do not silently auto-delete the exited session, because its final output and exit
status are useful history. Do not automatically create a replacement shell merely
because `exit` was intentional; present a clear create action and keep the rest
of the application usable. Preserve the existing persistence format unless the
reproduction proves that a format change is required; this bug fix must not add a
new session-format migration merely to retain closed history across a restart.

Required automated coverage:

- backend integration test that starts a real shell session, writes `exit\n`,
  observes one terminated status, and eventually sees `closed=true` with exit
  code `0`;
- backend race tests for delete-during-exit, disconnect-during-exit, and
  shutdown-during-exit;
- API tests proving a closed session can be listed and deleted and a new session
  can be created afterward;
- frontend unit tests for closed active-session reconciliation and prevention of
  reconnect/input/resize loops;
- Playwright flow for exiting one of two sessions and exiting the last live
  session, followed by creating and typing in a replacement terminal;
- assertions for no uncaught page error, no repeated WebSocket retry storm, and
  no backend goroutine/worker-session leak.

### 7.5 Container-aware top monitoring

This feature is feasible without privileged mode, host mounts, shell commands,
Kubernetes API access, or additional RBAC. The current deployed Roaminal Pod was
verified on 2026-08-07 with cgroup v2:

```text
cpu.max                  200000 100000     # 2 CPU hard limit
memory.max               2147483648        # 2 GiB hard limit
memory.current           about 33 MiB
memory.current-inactive_file about 31.8 MiB
kubectl top              about 30 MiB
/proc/meminfo MemTotal   about 16 GiB      # host value; incorrect for UI
```

The design follows the Linux kernel's
[cgroup v2 interface](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html)
and Kubernetes
[container resource semantics](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/).

#### Scope and truthfulness

Read the current process's cgroup, which accounts for the Go server, Node worker,
all Bash/PTYS, and their descendants. The current Deployment has one application
container, so container-cgroup usage is the Roaminal workload's Pod usage. If a
sidecar is added later, these values cease to be whole-Pod aggregates; then the
UI/API must say `container` or a separate Kubernetes/cAdvisor integration must
be designed. Never claim an inaccessible multi-container Pod aggregate.

Support cgroup v2 as the authoritative source. When cgroup v2 or a required
controller file is unavailable, return `available=false` and render `N/A` for
that metric. Do not fall back to `/proc/stat`, `/proc/meminfo`, `free`, `top`,
Node `os.*`, or host capacity and label it as container/Pod data.

#### Cgroup discovery

Do not hard-code a nested Kubernetes path. Implement a small read-only cgroup
reader behind filesystem and clock interfaces:

1. parse the unified entry from `/proc/self/cgroup`;
2. parse `/proc/self/mountinfo` and locate the `cgroup2` mount, including its
   mount root and mountpoint;
3. safely resolve the current cgroup relative to that mount root;
4. reject paths that escape the mountpoint;
5. read controller files from the resolved directory.

This must work both when a cgroup namespace exposes the current container as
`0::/` (the current Pod) and when `/proc/self/cgroup` exposes a nested path.

#### CPU calculation

- Read cumulative `usage_usec` from `cpu.stat`.
- Sample on one backend-owned 1-second ticker using monotonic elapsed time;
  heartbeat requests only read the cached snapshot.
- Compute `usageCores = delta(usage_usec) / delta(wall_usec)`.
- Parse `cpu.max` as `<quota> <period>`; finite capacity is `quota/period` and
  may be fractional.
- Parse `cpuset.cpus.effective` range syntax and use its CPU count as another
  capacity ceiling when present.
- Effective capacity is the minimum finite positive quota and cpuset capacity;
  if quota is `max`, use cpuset capacity; if neither is finite, expose capacity
  and percentage as unavailable rather than using host CPU count.
- Compute `usagePercent = usageCores / capacityCores * 100` for display and
  clamp only the visual progress bar, not the raw diagnostic value.
- Treat the first sample and counter reset as unavailable, never as a fake 0%.
- Optionally expose throttling diagnostics from `nr_periods`, `nr_throttled`,
  and `throttled_usec`, but do not make them required top-bar fields.

#### Memory calculation

- Read charged memory from `memory.current`.
- Read `inactive_file` from `memory.stat` and compute
  `workingSetBytes = max(0, memory.current - inactive_file)` so the primary UI
  remains comparable to Kubernetes/cAdvisor working-set reporting.
- Read `memory.max` as the enforced limit. The string `max` means no finite
  limit, not zero.
- With a finite limit, compute available bytes and percentage from working set
  and limit; clamp negative available bytes to zero.
- Expose both `currentBytes` and `workingSetBytes`, because `memory.current` is
  the actual cgroup charge while working set is the compact operational view.
- Read `oom`/`oom_kill` counters from `memory.events` for diagnostics, but do
  not add alarm behavior in this refactor.

#### Backend API contract

Extend the existing heartbeat system data rather than creating another endpoint.
Keep existing fields for compatibility while correcting their resource scope and
add explicit nullable fields:

```json
{
  "system": {
    "resourceScope": "cgroup-v2",
    "resourcesAvailable": true,
    "cpu": {
      "usageCores": 0.13,
      "capacityCores": 2,
      "usagePercent": 6.5
    },
    "memory": {
      "currentBytes": 33456128,
      "workingSetBytes": 33333248,
      "limitBytes": 2147483648,
      "usagePercent": 1.55
    }
  }
}
```

Nullable capacity, limit, and percentages represent unlimited/unavailable state;
do not encode those states as zero. Existing memory `totalBytes`, `usedBytes`,
and `freeBytes` become compatibility aliases for finite limit, working set, and
remaining limit. Existing CPU `usagePercent` uses the cgroup calculation. Host
model/clock values may remain diagnostic API fields but must not be presented as
container capacity.

The collector owns one ticker and cached immutable snapshot, supports clean
shutdown, and never samples once per connected browser. A slow or malformed
pseudo-file must not block heartbeat indefinitely; bounded file reads and normal
error propagation degrade the metric to unavailable without degrading terminal
service health.

#### Frontend presentation

Use Tabminal as an information-density reference, not as a field-for-field copy.
The Roaminal top bar must show:

- connection state and Pod/container hostname;
- CPU percentage plus a fixed-width progress indicator and effective core
  capacity in an accessible label/tooltip;
- memory working-set/limit plus percentage and a fixed-width progress indicator;
- Roaminal process uptime;
- terminal count and heartbeat round-trip latency;
- the existing persistence warning when applicable.

Do not show FPS, host CPU clock, or host uptime. Kernel and Pod IP can remain in
the heartbeat API but are lower-priority UI details.

Responsive behavior is fixed:

- wide desktop: hostname, CPU, memory, uptime, terminals, and RTT;
- tablet/narrow desktop: CPU, memory, terminals, and RTT;
- phone: connection, CPU percentage, memory percentage, and persistence warning;
- no text overlap, horizontal page scrolling, layout shift, or controls pushed
  off-screen at any supported viewport.

Use semantic text plus progress bars so color is never the only signal. Render
unlimited/missing metrics as `N/A` or `unlimited`, never `0%`. Escape all text;
do not use `innerHTML` string templates for metric values.

#### Monitoring tests

- table-driven cgroup discovery fixtures for namespace-root and nested paths;
- parsing tests for fractional/unlimited CPU quota, cpuset ranges, finite/max
  memory, malformed files, missing controllers, and path traversal attempts;
- fake-clock delta tests for first sample, normal use, counter reset, and
  concurrent heartbeat reads;
- memory tests including `inactive_file > current` clamping;
- API tests for finite, unlimited, and unavailable JSON without `NaN`/`Inf`;
- frontend tests for formatting, responsive field priority, progress clamping,
  and unavailable state;
- deployed comparison against direct cgroup reads and `kubectl top` within an
  explicitly documented sampling tolerance;
- Playwright screenshots at every configured viewport with overlap checks.

## 8. ForgeKit lint policy and source decomposition

Use the same policy locally and in CI. Root `lint.yaml` must include Go,
TypeScript, TSX, JavaScript module, and shell source while excluding generated
and downloaded content:

```yaml
include:
  - "**/*.go"
  - "**/*.ts"
  - "**/*.tsx"
  - "**/*.mjs"
  - "**/*.sh"

exclude:
  - "**/node_modules/**"
  - "**/dist/**"
  - "**/build/**"
  - "**/test-results/**"
  - "**/playwright-report/**"

max_lines_by_ext:
  .go: 250
  .ts: 300
  .tsx: 300
  .mjs: 300
  .sh: 200
```

Do not weaken `.go` to the current 991-line baseline. Split large files within
their existing packages without changing behavior:

- `config`: types/defaults, file loading, CLI parsing, environment parsing, and
  validation;
- `auth`: types, challenge/login, refresh/session lifecycle, and rate limiting;
- `persistence`: layout/store, auth persistence, session metadata, snapshots,
  and migration;
- `server`: router/middleware, auth handlers, session handlers, heartbeat/status,
  and shared response helpers;
- `terminal`: manager lifecycle, session lifecycle, client transport, PTY I/O,
  execution/title tracking, and restore;
- `worker`: protocol framing, client lifecycle, request queue, and snapshot
  operations;
- `monitor`: collector lifecycle/cache, cgroup discovery, CPU parsing/sampling,
  and memory parsing;
- `cmd/roaminal` may remain intact if it stays below the limit.

Keep package names and unexported ownership boundaries where possible. Splitting
a file is not permission to redesign APIs.

The `commands` list in `lint.yaml` must run, in deterministic order:

1. `bash -n setup/forgekit.sh` and syntax checks for repository shell scripts;
2. `gofmt` cleanliness for `backend/`;
3. `go -C backend test ./...`;
4. `go -C backend test -race ./...`;
5. `go -C backend vet ./...`;
6. `npm --prefix terminal-worker ci`;
7. terminal-worker lint and tests;
8. `npm --prefix frontend ci`;
9. frontend ESLint, typecheck, unit tests, and production build.

Do not put Playwright against a live deployment in `forgekit lint`; that is a
separate final integration gate. Do not make lint depend on Kubernetes or secret
credentials.

## 9. ForgeKit bootstrap and repository rules

Adapt `k8s-at-home/setup/forgekit.sh` for this repository with these fixed
properties:

- minimum version: `0.6.1`;
- preferred version: `0.6.1`;
- upstream: `ben-wangz/forgekit`;
- supported platforms: Linux/macOS on amd64/arm64;
- checksum verification is mandatory when `sha256sum` is available;
- destination: `build/bin/forgekit`;
- the script determines the repository root from `BASH_SOURCE` and works from
  any current directory;
- environment overrides retain the reference names
  `FORGEKIT_MIN_VERSION`, `FORGEKIT_BEST_VERSION`, `FORGEKIT_REPO`, and
  `FORGEKIT_DOWNLOAD_BASE`.

Add `/build/` to `.gitignore`. Add root `AGENTS.md` rules stating that agents
must not manually edit `container/VERSION`, must use ForgeKit for product version bumps,
must run ForgeKit lint after applicable code changes, and must not treat private
npm manifest versions as release versions.

Add a concise version section to README and a detailed `docs/releasing.md` with:

- bootstrap and query commands;
- patch/minor/major bump commands;
- required review of the version-only diff;
- `roaminal-v<semver>` tag format;
- the rule that a release tag is created only after the lint workflow succeeds
  on the version commit;
- a clear note that automated registry publication is not introduced by this
  refactor.

## 10. Container and deployment changes

Move the production build definition to `container/Containerfile`. It must use
four stages: frontend builder, worker dependencies, backend builder, and runtime.

Required changes:

- replace all `web/` paths with `frontend/`;
- build frontend to `frontend/dist` and copy it directly into
  `/opt/roaminal/frontend` in the runtime image;
- build Go with `go -C backend` and the `/backend` module path;
- copy `backend/shell` to `/opt/roaminal/shell`;
- keep `terminal-worker/` at its existing source path and existing runtime path;
- accept `ARG ROAMINAL_VERSION=dev` in relevant stages;
- inject that argument into `buildinfo.Version`;
- set `org.opencontainers.image.version` from the same argument;
- keep current pinned base-image digests, UID/GID, read-only compatibility,
  healthcheck, volumes, entrypoint, and license copies;
- set `ROAMINAL_FRONTEND_DIR=/opt/roaminal/frontend`.

### 10.1 PID 1, signals, and child reaping

An init process is mandatory because Roaminal intentionally launches a Node
worker, interactive Bash processes, and arbitrary terminal descendants. Preserve
the exec-form entrypoint exactly in intent:

```dockerfile
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/roaminal"]
```

Do not wrap this in `sh -c`, do not remove `tini`, and do not require users to
add Podman `--init` on top of the image's own init. The checked-in Kubernetes
Deployment currently does not override `command` or `args`, so it must continue
to use the image entrypoint. Keep `deploy/kubernetes/` unchanged.

The required process and signal model is:

```text
Kubernetes/Podman SIGTERM
          |
          v
PID 1 tini (forwards signals; reaps adopted orphans)
          |
          v
Go roaminal (graceful shutdown coordinator)
     |                         |
     v                         v
Node worker PGID          Bash session PGID(s)
```

The implementation must preserve or enforce these properties:

- `tini` is PID 1 and the Go process is its direct child;
- Go handles `SIGTERM` and `SIGINT`, stops accepting new work, shuts down HTTP,
  snapshots terminal state, and coordinates children before exiting;
- the worker remains in a dedicated process group and has parent-death
  protection;
- every PTY Bash remains a session/process-group leader and has parent-death
  protection;
- terminal close/delete/shutdown signals the negative PGID, not only the Bash
  leader PID, so foreground/background jobs are included;
- graceful termination sends `SIGTERM`, waits with a bounded deadline, then
  sends `SIGKILL` to a process group that is still alive;
- worker shutdown requests a clean protocol shutdown first, then kills its
  process group on timeout;
- shutdown is idempotent across concurrent shell exit, user delete, worker
  failure, HTTP shutdown, and Kubernetes termination;
- all internal deadlines complete comfortably inside the unchanged Kubernetes
  `terminationGracePeriodSeconds: 30`; no child cleanup starts after the Go
  process has already exited;
- the implementation never uses host PID traversal, privileged operations, or
  writable `cgroup.kill` to terminate sessions.

`kill` and init solve different problems. A shell's `kill` builtin must continue
to signal processes owned by UID 1000 even though Linux capabilities are
dropped; `CAP_KILL` is not required for same-UID processes. `tini` is responsible
for PID 1 forwarding/reaping, while Roaminal remains responsible for its worker
and PTY process groups.

Expected command behavior:

- `sleep 300 &` followed by `kill <pid>` terminates that job normally and leaves
  its terminal usable;
- `kill -TERM $$`, `kill -KILL $$`, and shell `exit` all converge on the closed
  terminal lifecycle in section 7.4 without freezing the rest of the app;
- closing a terminal that owns a background job terminates the complete session
  process group within the bounded escalation window;
- signaling PID 1 in a disposable container initiates whole-service shutdown;
  this is expected because terminal users share the trusted UID with Roaminal;
- a forced Go-process death cannot leave the worker or Bash leader alive, and
  orphaned exited descendants are reaped by `tini` rather than accumulating as
  zombies.

Do not assume that `tini` alone kills every descendant: it forwards to its direct
child and reaps adopted children, while the Go shutdown coordinator must still
terminate the independently created worker and Bash process groups.

The build context remains the repository root because the image assembles all
three modules. Root `.containerignore` is the sole exception to the
container-directory rule because Podman reads ignore rules from the build
context; document this reason and update its paths. Do not introduce a second
Containerfile at root.

For ForgeKit container planning/building use:

```sh
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
FORGEKIT_BIN="$(bash "$PROJECT_ROOT/setup/forgekit.sh")"
ROAMINAL_VERSION="$("$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal --git)"

CONTAINER_REGISTRY=registry.example.invalid \
IMAGE_NAME=roaminal \
BUILD_ARG_ROAMINAL_VERSION="$ROAMINAL_VERSION" \
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" publish container build \
  --container-dir container \
  --module roaminal \
  --context "$PROJECT_ROOT" \
  --no-push
```

Release builds use `version get roaminal` and ForgeKit `--semver`; development
builds use `version get roaminal --git`. The build argument, image tag, OCI
label, `roaminal --version`, and `/api/version` must resolve to the same value.

Update `.containerignore`, `.gitignore`, README, and container build commands for
the new paths.

`deploy/kubernetes/` is deliberately out of scope for structural or content
changes in this refactor. Do not move it, convert it to a Helm chart, add a chart
directory, register a ForgeKit chart target, rename resources, or clean up its
existing labels/placeholders. Deployment verification may update the live
Deployment image with `kubectl set image`; it must not rewrite the checked-in
manifests. Chart packaging can be reconsidered only after core product behavior
has matured.

## 11. CI workflow

Add `.github/workflows/lint.yaml`, modeled on the reference repository:

- trigger on pull requests and pushes to `main`;
- checkout with `actions/checkout@v4`;
- install Go using `backend/go.mod`;
- install Node.js `24.13.1` with npm cache paths for both Node modules;
- bootstrap ForgeKit through `setup/forgekit.sh` and print its version;
- run exactly:

```sh
"$FORGEKIT_BIN" --project-root "$GITHUB_WORKSPACE" lint \
  --config "$GITHUB_WORKSPACE/lint.yaml"
```

Do not duplicate lint commands as separate workflow steps. The checked-in
`lint.yaml` is the single lint workflow definition.

## 12. Implementation phases and atomic commits

Every commit after the initial documentation commit must pass the checks relevant
to the files it changes. Use these commit boundaries:

1. `docs: retire MVP planning and define repository refactor`
   - include the already requested MVP-plan deletion, corrected historical
     Tabminal wording, README dead-link removal, and this plan;
   - do not include source moves.
2. `refactor(backend): decompose packages for lint boundaries`
   - split oversized Go files in place;
   - preserve the current module path and behavior for this commit;
   - run Go unit/race/vet gates.
3. `refactor(repo): isolate backend frontend and worker modules`
   - perform all directory moves;
   - change the backend module path/imports;
   - replace embedded assets with the static-directory adapter;
   - localize module-owned fixtures;
   - move/update Containerfile, update ignore files and README development
     commands, and update path-sensitive configuration/deployment/troubleshooting
     documentation so the commit is buildable and its documented commands work.
4. `fix(container): harden init and signal lifecycle`
   - preserve `tini` as PID 1 and add explicit stop-signal semantics;
   - add bounded TERM-to-KILL escalation and idempotent process-group cleanup;
   - include the disposable-container and process-tree tests from section 10.1.
5. `fix(terminal): handle normal shell exit`
   - add the reproduction first, then correct backend/frontend lifecycle behavior;
   - include backend race coverage, frontend state tests, and focused Playwright
     coverage from section 7.4.
6. `feat(monitor): report cgroup-scoped resources`
   - replace host CPU/memory usage with the cgroup v2 collector;
   - extend heartbeat types and build the responsive top status UI;
   - include parser, sampler, API, frontend, and viewport tests from section 7.5.
7. `build(forgekit): add lint and version control`
   - add `container/VERSION`, `version-control.yaml`, `lint.yaml`, bootstrap
     script, build-info injection, private npm version normalization, and
     `AGENTS.md`;
   - verify ForgeKit read-only version commands and full ForgeKit lint.
8. `ci: enforce forgekit lint`
   - add the GitHub Actions workflow only.
9. `docs: document modular development and releases`
   - add the README ForgeKit/version section;
   - add `docs/releasing.md`;
   - reconcile licensing-notice paths and remove current-state MVP wording
     outside the intentionally unchanged `deploy/kubernetes/` manifests.

If path coupling makes commit 3 too broad to review, it may be split into
`refactor(frontend)`, `refactor(backend)`, and `build(container)` commits, but no
intermediate commit may leave the default branch unbuildable.

## 13. Verification gates

### 13.1 Baseline before edits

Run and record:

```sh
go test ./...
go test -race ./...
go vet ./...
npm --prefix terminal-worker test
npm --prefix terminal-worker run lint
npm --prefix web test -- --run
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run build
```

### 13.2 Module gates after the move

Run from repository root:

```sh
go -C backend test ./...
go -C backend test -race ./...
go -C backend vet ./...
npm --prefix terminal-worker test
npm --prefix terminal-worker run lint
npm --prefix frontend test -- --run
npm --prefix frontend run typecheck
npm --prefix frontend run lint
npm --prefix frontend run build
```

Also verify the shell scripts with `bash -n` and build the backend with a known
temporary version using `-ldflags -X`.

### 13.3 ForgeKit gates

```sh
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
FORGEKIT_BIN="$(bash "$PROJECT_ROOT/setup/forgekit.sh")"
"$FORGEKIT_BIN" --version
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal --git
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" lint \
  --config "$PROJECT_ROOT/lint.yaml"
```

`version bump` must not be run against the real worktree merely as a test. Test
mutation behavior in a temporary fixture repository if needed.

### 13.4 Static serving and container gates

Automated backend tests must cover:

- `/` and `/index.html` with no-cache semantics;
- hashed `/assets/*` immutable caching;
- ordinary static-file caching;
- missing `index.html` startup failure;
- missing asset `404`;
- traversal rejection;
- API routing taking precedence over static files.

Then build and run the container. Verify:

- `roaminal --version` equals the injected build value;
- `GET /api/version` returns the same value;
- OCI `org.opencontainers.image.version` equals that value;
- `/healthz` is healthy;
- the frontend loads from the runtime static directory;
- the terminal worker handshake and terminal restore still work;
- `exit` preserves a readable closed session while create/delete/switch/input on
  the rest of the application remains usable;
- heartbeat CPU/memory values match direct reads from the container cgroup
  within the documented sampling tolerance;
- PID 1 is `tini`, Go is its direct child, and worker/Bash processes have the
  expected independent process groups;
- `podman stop --time` reaches clean shutdown before its deadline with no
  remaining child processes or zombies;
- session close kills both cooperative and TERM-ignoring background jobs through
  bounded TERM-to-KILL escalation;
- the image still runs as UID/GID 1000 with a read-only root filesystem.

### 13.5 Kubernetes and browser gate

Deploy the final immutable image to namespace `develop`, wait for rollout, and
use only:

```text
http://roaminal.develop.svc.cluster.local:9846
```

Run the full Playwright viewport matrix from `frontend/`. Do not use
`kubectl port-forward`. Confirm the Pod is Ready with zero unexpected restarts,
the Service address is unchanged, `/healthz` succeeds directly, login works,
terminal creation/switching/input works, hover preview cleanup works, and no
page/console errors are introduced. Include the normal-exit scenarios and verify
the monitoring fields against direct cgroup reads and `kubectl top` without
granting the Pod new permissions.

## 14. Stop conditions

Stop and report evidence only when one of these conditions is true:

- user-owned concurrent changes overlap the same files and cannot be preserved
  with an unambiguous merge;
- the pre-refactor baseline has a reproducible failure that makes equivalence
  impossible to judge;
- ForgeKit `v0.6.1` cannot be obtained from either configured source or fails
  checksum verification after the documented fallback attempts;
- moving away from embedded assets reveals an undocumented deployment contract
  that would require changing external API or persistence behavior;
- required deployment credentials or cluster access are unavailable after local,
  container, and read-only cluster diagnostics are exhausted;
- the direct Service DNS is unavailable from the execution environment after
  verifying namespace, Service, endpoints, and network resolution. Do not work
  around this with port forwarding.

Ordinary test failures, path errors, lint violations, or build errors introduced
by the refactor are not stop conditions; fix them and continue.

## 15. Definition of Done

- [ ] `backend/` contains all Go backend code, its Go module, and shell runtime
      resource.
- [ ] `frontend/` contains all browser source, configs, tests, and frontend-owned
      fixtures.
- [ ] `terminal-worker/` retains its top-level name and owns its fixtures.
- [ ] Root `cmd/`, `internal/`, `shell/`, and `web/` no longer exist.
- [ ] No generated frontend bundle is tracked and no Go package embeds frontend
      assets.
- [ ] Backend, frontend, and terminal worker install/test/build independently.
- [ ] `container/Containerfile` is the sole production assembly boundary and no
      root Containerfile remains.
- [ ] `container/VERSION` is the only product release version.
- [ ] `version-control.yaml` registers exactly one `roaminal` container target.
- [ ] `deploy/kubernetes/` remains ordinary YAML with no Helm chart and has no
      content changes from this refactor.
- [ ] `setup/forgekit.sh` reliably resolves ForgeKit `v0.6.1` from any cwd.
- [ ] ForgeKit version query and Git-version query both succeed.
- [ ] `forgekit lint` passes from a fresh dependency state.
- [ ] All included source files satisfy the documented line limits.
- [ ] GitHub Actions invokes the same ForgeKit lint configuration.
- [ ] Build argument, image tag, OCI label, CLI version, and API version agree.
- [ ] Existing API, auth, terminal, persistence, worker, and UI tests pass.
- [ ] Executing `exit` preserves a closed read-only history session and never
      makes the rest of Roaminal inoperable or start a reconnect loop.
- [ ] Cgroup v2 CPU/memory metrics represent the current Roaminal container,
      degrade honestly when unavailable, and do not use host `/proc` capacity.
- [ ] The responsive top bar shows CPU, memory, uptime, terminal count, RTT, and
      existing health state without overlap at supported viewports.
- [ ] `tini` is PID 1, forwards stop signals to Go, and reaps adopted children.
- [ ] Go terminates worker and PTY process groups gracefully, escalates to
      `SIGKILL` on deadline, and exits within the Kubernetes grace period.
- [ ] Shell `kill`, `kill $$`, terminal close/delete, Podman stop, and Kubernetes
      rollout lifecycle tests pass without leaked processes or zombies.
- [ ] The container passes health, security-context, and static-serving checks.
- [ ] The `develop` deployment and direct-Service Playwright matrix pass without
      port forwarding.
- [ ] README, configuration, deployment, troubleshooting, licensing notices, and
      release documentation contain no stale source paths or current-state MVP
      wording.
- [ ] Worktree changes are committed in the atomic sequence above and pushed only
      when the user has authorized pushing.
