# PW-REL-008: Client request lifecycle and stability

Priority: P0. Capabilities: core Helm release; add SSH/tmux only for Agent and
FileSystem steps. Viewport: desktop, with a phone smoke pass.

## Procedure and assertions

1. Complete the mandatory Helm deployment and browser diagnostics gates before
   creating a context. Record every request's method, normalized path, start
   and end time, owning view/action, and maximum concurrent count. Use a fresh
   login and reset the browser notification opt-in to off.
2. Observe at least 30 one-second heartbeat cycles while switching Settings
   sections, Sessions, Terminal, FileSystem, Message Center, and the Agent
   dialog. With notifications disabled, `/api/v2/notifications/preferences`
   is requested once for the stable authenticated lifecycle, and neither
   `/api/v2/notifications/config` nor subscription registration is requested.
   No heartbeat, render, workspace switch, or dialog refresh creates another
   preference request.
3. In a separate permission-granted context with the authenticated browser
   notification opt-in enabled, observe the same cycles. Config, preference,
   and subscription synchronization each occur once per authentication
   identity. Replacing the access/refresh token through the normal auth flow
   causes exactly one new lifecycle synchronization, not a render loop.
4. Open an Agent dialog and verify it does not fetch or edit notification
   preferences. Use `Manage in Settings` to open the shared Notifications
   section and verify the lifecycle still performs no repeated GET, unbounded
   polling, or overlapping preference request.
5. With Message Center mounted, record the baseline `/api/v2/messages` request.
   Unchanged heartbeat revisions produce no additional history request. Change
   the heartbeat message revision twice while one request is delayed and verify
   one coalesced follow-up after the first completes. A failed history request
   creates one bounded retry timer; repeated failures do not create a timer
   storm, and a successful retry clears it.
6. Switch FileSystem directories and trigger manual/automatic refreshes while
   one listing or content request is delayed. Verify requests owned by a
   directory or preview do not duplicate, stale responses cannot replace newer
   state, and unmounting the workspace cancels timers and leaves no request
   associated with the removed view.
7. Close the application view, sign out, and unmount the notification and
   Message Center owners while requests are pending. Verify timers are cleared,
   late responses are ignored, no state update produces an unhandled rejection,
   and no request remains in flight after the cleanup observation window.
8. Repeat the render/switch/heartbeat sequence at phone width. Verify no
   request burst accompanies native keyboard open/close, input viewport
   adjustment, monitor disclosure, virtual-keyboard switching, fullscreen
   capability probing, or appearance sample updates.
9. Inspect the final diagnostics collection. There must be no
   `net::ERR_INSUFFICIENT_RESOURCES`, uncontrolled request burst, duplicate
   same-role request, unexpected `4xx`/`5xx`, console warning/error,
   unhandled rejection, failed WebSocket request, or socket left open after
   cleanup. Expected negative responses must be correlated with the action that
   caused them.

## Pass gate and cleanup

Report a request table with lifecycle owner, count, maximum concurrency, and
cleanup result. Fail if any stable-auth endpoint is tied to an app-shell render
or one-second heartbeat. Restore notification storage, close all contexts, and
leave the Helm release, definitions, connection instances, and remote fixture
unchanged.
