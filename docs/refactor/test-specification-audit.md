# Current-code test specification audit

Status: implementation complete; full browser regression execution pending. This
file is temporary and must be removed after the remaining verification is
complete.

## Objective

Bring the authoritative browser regression specifications in `tests/` in line
with the current Roaminal code, add coverage for recently exposed regression
classes, and keep deterministic behavior covered by colocated unit tests. The
specification and deterministic-test portions of this plan are implemented;
the final browser execution remains environment-dependent.

The reviewed baseline contained 44 browser cases plus the shared SSH/tmux
fixture. The review covered the current HTTP and WebSocket routers, app shell,
connection-instance layout, FileSystem, Agent local-state synchronization,
Message Center, browser notifications, fullscreen handling, mobile input, and
their existing unit tests.

## Audit findings

### Invalid current specifications

1. `security/01-browser-security.md` contradicts the Web Push implementation.
   It currently forbids every Service Worker and allows only authentication and
   active-selection local storage. After explicit notification opt-in, the
   product intentionally registers `/roaminal-sw.js`, stores notification
   deduplication claims in IndexedDB, and may persist these browser preferences:

   - terminal appearance;
   - FileSystem auto-refresh interval;
   - connection-group collapse state scoped by login session;
   - Virtual Keyboard disclosure state scoped by login session;
   - system-notification opt-in.

   The case must use a phase-specific allowlist. Before opt-in there must be no
   Service Worker registration. After opt-in exactly the Roaminal notification
   worker is allowed. Its IndexedDB records may contain only a message ID and
   expiry metadata. Cache Storage, cookies, terminal content, connection
   credentials, notification bodies, and other unexpected persistence remain
   forbidden.

2. `workspace/02-sidebar-selection-and-preview.md` and
   `workspace/13-filesystem.md` still use the Agent robot to return from
   FileSystem to Terminal. The current UI deliberately separates the Terminal
   extension from the Agent-status robot. The Terminal icon changes workspace
   mode; the robot always opens Agent details and never changes mode.

3. Terminology is stale in several cases. Replace `auth-session` with `login
   session`, and replace the unqualified "creating a session" in SSH key
   generation with "creating a connection instance". Preserve `tmux session`
   and protocol error strings such as `invalid session id` where they are exact
   runtime concepts or messages.

4. The coverage index omits the FileSystem case and does not expose
   connection-instance groups or per-target Agent notification preferences as
   independent coverage. Workspace case identifiers also switch between
   `PW-WORK`, `PW-WORKSPACE`, and `PW-WS`; normalize them to `PW-WORK` without
   renaming files.

### Missing regression boundaries

1. Browser notification synchronization has no browser-level request-lifecycle
   assertion. A stable login must not refetch notification configuration or
   preferences on every one-second heartbeat, Message Center update, workspace
   switch, or app-shell render. This is the regression class that can exhaust
   Chromium request resources and produce `net::ERR_INSUFFICIENT_RESOURCES`.

2. Message synchronization has controller tests but no hook- or browser-level
   assertion that one baseline request is followed only by revision-driven
   requests. Concurrent heartbeat changes must coalesce to one follow-up, and a
   failed request must keep only one bounded retry timer.

3. Connection-instance group behavior is embedded in the already broad sidebar
   and preview case. It does not fully prove server revision conflicts,
   optimistic rollback, independent layouts for two login sessions, or the
   empty-group delete retry. Group behavior needs a dedicated case.

4. Agent notification preferences have only incidental coverage. The UI and
   API key preferences by stable connection definition plus tmux session name,
   not by connection instance. Tests must prove default-off behavior, parent
   and child toggle gating, persistence, target sharing, target isolation,
   failed-write rollback, and transition filtering.

5. The authorization case predates the Agent, Message Center, notification,
   connection-group, and FileSystem route families. These protected routes need
   representative unauthenticated, wrong-Origin, malformed-body, and ownership
   checks without duplicating every functional case.

6. Terminal appearance coverage does not directly guard the previous preview
   churn regression. The sample must retain one xterm instance and stable
   content across heartbeat, message, notification-state, and unrelated app
   renders. Only an appearance draft change may update it.

7. The Agent artwork contract lacks an unsupported-target test. Product design
   assigns sleeping to an uninitialized supported target and confusing to an
   unsupported or unavailable target. The current mapping checks
   `uninitialized` before support, so a local or non-tmux connection can render
   sleeping while its tooltip says unavailable. Add a focused unit assertion
   and preserve the design contract in the browser case; fix product mapping if
   the new assertion exposes this mismatch.

8. The notification Service Worker has functional deduplication tests, but the
   browser case does not verify that repeated worker click messages cause one
   navigation/read mutation, or that stored deduplication data never includes
   notification presentation text or connection metadata.

## Specification changes

### Shared contract and indexes

Update `tests/playwright/README.md` to:

- list FileSystem, connection-instance groups, Agent notification preferences,
  and client request stability in the coverage index;
- name both supported WebSocket route families explicitly;
- define a reusable request-count rule: record method, normalized path,
  start/end time, and maximum concurrent count for polled or lifecycle-owned
  endpoints;
- state that no `net::ERR_INSUFFICIENT_RESOURCES`, uncontrolled request burst,
  or request remaining in flight after cleanup is allowed.

Keep `tests/AGENTS.md`, `tests/README.md`, and the fixture focused on execution
rules. Do not introduce a second checked-in Playwright suite.

### Correct existing cases

Update these existing specifications:

| Case | Required correction |
| --- | --- |
| `auth/01-login.md` | Make its storage assertion explicitly apply to the fresh-login phase; it must not conflict with later opt-in preferences. |
| `keys/02-generation.md` | Use connection-instance terminology. |
| `reliability/03-backend-restart-persistence.md` | Use login-session terminology and preserve the current non-resurrection contract. |
| `security/01-browser-security.md` | Replace the blanket worker/storage prohibition with the phase-specific allowlist above. |
| `security/02-http-websocket-authorization.md` | Add representative checks for all current protected route families and use current terminology. |
| `workspace/02-sidebar-selection-and-preview.md` | Keep selection, search, and preview behavior; move group lifecycle detail to the new group case; use Terminal and robot controls according to their separate roles. |
| `workspace/12-terminal-appearance.md` | Normalize the case ID and add sample-instance stability assertions. |
| `workspace/13-filesystem.md` | Normalize the case ID and use the Terminal extension to leave FileSystem. The robot must open Agent details without changing the open FileSystem state. |
| `workspace/14-codex-agent-status.md` | Normalize the case ID; assert artwork source/alternative text for all five visual states and the unsupported-target distinction. |
| `workspace/15-agent-messages.md` | Normalize the case ID and add single-delivery behavior for repeated Service Worker click messages. |
| `workspace/16-browser-capabilities.md` | Normalize the case ID; move detailed preference identity behavior to the new preference case and retain permission, subscription, fullscreen, and end-to-end delivery coverage. |

### Add dedicated browser cases

Add `workspace/17-connection-instance-groups.md` with these boundaries:

- new connection instances always append to `Ungrouped`;
- create, trim, case-insensitive uniqueness, rename, collapse, reorder, move,
  move-all, and empty-only deletion;
- pointer and keyboard reorder parity, including search disabling mutations;
- 10-member named-group limit and unlimited `Ungrouped`;
- server persistence across reload and independent layouts across login
  sessions, while collapse state remains local and login-session scoped;
- two-browser stale-revision conflict, visible optimistic rollback, one safe
  refresh/retry for an empty-group delete, and no duplicate mutation;
- exit/retirement reconciliation without reordering survivors.

Add `workspace/18-agent-notification-preferences.md` with these boundaries:

- controls appear only for a supported SSH/tmux target and default to off;
- enabling the parent does not silently enable transition children;
- `running -> relax` and `running -> error` can be selected independently;
- two connection instances for the same connection definition and tmux session
  share one preference, while another session or definition is isolated;
- reload, another login session, and backend restart preserve server state;
- failed PUT restores the prior UI state and shows one error;
- disabled targets still create normal Message Center rows but no browser
  notification;
- preference reads/writes contain no endpoint, prompt, transcript, or provider
  runtime identifiers.

Add `reliability/08-client-request-stability.md` with these boundaries:

- observe at least 30 heartbeat cycles while switching pages, connections,
  workspace tools, FileSystem/Terminal mode, Message Center, and Agent dialog;
- with notifications disabled, preference synchronization occurs once per
  stable authentication identity and no push-config/subscription call occurs;
- with notifications enabled and granted, config/subscription synchronization
  occurs once per authentication identity, not per render;
- an Agent dialog may perform its one explicit preference refresh, then stays
  quiet while open;
- message history loads once at baseline and only when heartbeat message
  revision changes; unchanged heartbeat payloads produce no message request;
- delayed notification and message responses never overlap with another request
  of the same role; a transient failure creates one bounded retry and recovers;
- token rotation performs one new authentication-lifecycle synchronization;
- sign-out/unmount cancels timers and leaves no pending requests;
- browser diagnostics contain no insufficient-resource error, request storm,
  unhandled rejection, or unexpected `4xx`/`5xx`.

## Deterministic unit coverage

Add focused colocated unit tests before running the browser cases:

1. A hook-level test for `useBrowserNotifications` that rerenders with changing
   navigation callback identities while authentication is stable. Listener
   installation and preference/push synchronization must remain single, the
   latest callback must receive a click, auth replacement must synchronize once,
   and unmount must unsubscribe.
2. A hook-level test for `useMessages` covering unchanged heartbeat revisions,
   coalesced follow-up synchronization, one retry timer, and timer cleanup.
3. An Agent visual-state test for unsupported plus uninitialized, supported plus
   uninitialized, unavailable, stale, running, relax, and explicit error.
4. Extend notification-worker tests to prove duplicate message IDs and repeated
   click delivery do not create duplicate notification or navigation effects,
   and that deduplication records contain only bounded identity/expiry data.

Do not duplicate backend tests already covering message persistence,
notification preference persistence, Agent snapshot validation, and group
revision validation. Add server tests only when the expanded authorization
matrix exposes an untested route-specific contract.

## Implementation order

1. Correct the invalid security, terminology, control-role, case-ID, and index
   statements. Run a repository terminology and Markdown-link check.
2. Extract group lifecycle assertions from the sidebar case and add the three
   dedicated browser cases. Keep each precondition, mutation owner, cleanup,
   and expected negative response explicit.
3. Add deterministic frontend unit tests. If the unsupported Agent artwork test
   fails, correct the mapping rather than weakening the specification.
4. Run frontend tests and the full ForgeKit quality gate.
5. Deploy the exact candidate through the repository Helm Chart, record release
   metadata, and run P0 cases followed by the changed P1 cases.
6. Run the complete 47-case regression set on the required viewport
   matrix. Stateful SSH/tmux, restart, key, group-concurrency, Agent, and Web
   Push cases remain serial and use isolated fixtures.
7. Re-audit this document. Remove it only when every listed specification,
   deterministic test, browser case, cleanup, and diagnostics gate has passed.

## Completion criteria

- No browser specification contradicts the current UI, route surface, storage
  model, Agent state model, or Web Push lifecycle.
- The coverage index resolves every maintained case and all case IDs use the
  same naming convention.
- Every current protected route family has an explicit authorization owner.
- Group layout, notification preference identity, and request-lifecycle
  regressions have dedicated executable specifications.
- New deterministic unit tests pass, followed by the ForgeKit quality gate.
- The Helm-deployed full browser regression has no unexpected console warning,
  page error, failed request, HTTP error, WebSocket error, leaked secret,
  request burst, or incomplete cleanup.

## Verification record

- PASS: frontend unit tests, typecheck, lint, and production build.
- PASS: ForgeKit lint, including Go unit/race/vet, hook bundle, worker tests,
  Chart lint/render/validation, and workflow syntax.
- PASS: Markdown link and 47-case numbering checks.
- PASS: representative v2 HTTP/WebSocket smoke against the reachable `develop`
  Service/Ingress; that release was 0.3.8 while the candidate source is 0.3.15.
- PENDING: full 47-case browser execution against a candidate deployment. The
  repository stores AI-agent Markdown specifications rather than an executable
  Playwright runner, and the shared environment did not provide an isolated
  candidate deployment or the required SSH fixture alias/credentials for a
  safe full run.
