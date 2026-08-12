# PW-CONN-009: Pending tmux-launch cancellation and failure handling

Priority: P0. Capabilities: controllable SSH/tmux fixture. Viewport: desktop.
Run serially.

Fixture: use the [SSH and tmux codespace](../fixtures/ssh-codespace.md). Apply
the documented remote-state reset and use only the fixture-owned Host.

## Procedure and assertions

1. Delay tmux readiness in the fixture and Start. Verify one pending launch ID,
   one `/ws/connection-launches/:id` socket, a disabled/non-live contextual
   keyboard, and no premature instance card.
2. While pending, click Connections. A keepalive DELETE cancels that launch,
   its PTY/process is reaped, and no later heartbeat publishes it.
3. Repeat and reload during the pending handshake. `pagehide` sends the same
   cancellation semantics. After reload there is no orphan pending instance,
   repeated launch socket, or console error about a socket closing before it
   opened.
4. Repeat and sign out while pending. The launch is canceled before browser auth
   state is cleared.
5. Make preflight fail and make tmux terminate before publish in separate runs.
   The launch runtime closes, the workspace returns to a usable manager/empty
   state, and exactly one user-facing failure appears.
6. Start successfully immediately after each cancellation/failure to prove
   stale launch state does not block the next launch.

## Pass gate

Query active instances and, where the test harness can inspect the test state,
confirm no pending metadata or process remains. Expected cancellation requests
are allowed; `invalid session id`, early WebSocket-close console output, and
disposed-runtime exceptions are failures.
