# PW-WORK-020: Remote browser workspace

Priority: P1. Capabilities: remote-browser. Viewports: desktop, tablet
portrait, phone portrait, and phone landscape. Run against a Helm deployment
with `app.browserEnabled=true` and a fixture service reachable only from the
Roaminal Pod.

## Procedure and assertions

1. Complete the mandatory Helm deployment and browser diagnostics gates from
   the Playwright README before opening the application. Register console,
   page-error, failed-request, response, and WebSocket diagnostics. Confirm
   the fixture service is not resolvable from the test browser itself.
2. Log in and verify the globe control is the far-left item in the workspace
   rail, before Connections. Click it with no terminal connection. The main
   surface changes to the remote browser address form, the Connections drawer
   is collapsed, and no target request is made before an address is submitted.
3. Submit the fixture's HTTP address. Verify the target request, relative
   assets, redirects, cookies, and an application WebSocket originate from
   the Roaminal Pod. The test browser must see only the Roaminal origin and
   the streamed page surface; it must not issue requests to the fixture.
4. Verify the toolbar exposes Back, Forward, Reload, page identity, Copy,
   Paste, an icon-only Crown primary-client control, and an icon-only Back to
   terminal action. The page identity shows `Network: Roaminal` and opens the
   address dialog when clicked. There is no permanent browser address bar.
   Cancel leaves the current page unchanged. The active Crown has the
   accessible name `Primary client`.
5. Try invalid, credential-bearing, `file:`, and browser-internal addresses.
   Verify each is rejected without a target request.
6. Exercise pointer click, drag, wheel, keyboard input, password input,
   Chinese IME text, local clipboard paste, remote selection copy, and
   viewport resize. Verify Backspace, Delete, Enter, Tab, arrows, Home, End,
   PageUp, PageDown, Escape, Insert, function keys, modifier combinations,
   and printable text reach the fixture page with their key identity intact.
   Verify Ctrl/Cmd+V and the Paste control insert the local
   clipboard into the remote page, while Ctrl/Cmd+C and the Copy control place
   the remote selection in the local clipboard. With no remote selection,
   Ctrl/Cmd+C keeps its normal remote key behavior. Verify input reaches the
   fixture page, no terminal receives browser keystrokes, the canvas remains
   within the workspace, and no local file picker or download is opened.
   Repeated identical content-host measurements produce no extra resize
   request. When the host and remote page have different aspect ratios, the
   complete frame remains visible with letterboxing and pointer coordinates
   land on the same remote page location.
7. Switch repeatedly between Browser, Terminal, Connections, and Files while
   a terminal command is running. Browser page state and terminal process
   state remain intact. Settings preserves its unsaved-changes guard and
   returns to Browser even when there are no terminal connections.
8. Open a second Roaminal browser client against the same deployment. Verify
   it immediately receives the existing page URL, title, status, and frame
   after activating Browser, without submitting the address again. Both
   clients receive the same frames and page state. The first client whose
   valid resize is accepted has an active Crown; the other receives a
   `not_primary_client` response, changes to the inactive `Set as primary
   client` Crown, and sends no further automatic resize requests. Clicking the
   inactive Crown opens `Take over primary client?`; Cancel sends nothing.
   Confirming `Take over` sends one takeover resize using the latest measured
   dimensions. On success the new client becomes primary and later unique
   sizes are applied. A failed takeover leaves it inactive without a retry
   loop.
9. While the page is open, switch each client through Terminal, Connections,
   Files, and Settings, including a Settings remount and an unsaved-change
   guard. Returning to Browser shows the same URL, history, cookies, form
   values, and content. No workspace switch, client disconnect, refresh, or
   WebSocket cleanup sends a browser `close` command.
10. Click the icon-only `Close page` button from either client, including from
   a non-primary client while the other client is hidden or reconnecting.
   Verify both clients receive the global closed lifecycle event, clear the
   canvas and page metadata, and show the address form. Two simultaneous close
   clicks are harmless. Open a new URL and verify both clients show the new
   single page and no second worker or target exists.
11. Send delayed commands from the old page after reopening, including close,
   input, navigation, and dialog responses. They must be rejected or ignored
   with a resync, must not affect the new page, and must not close either
   WebSocket. Also close the browser WebSocket, refresh the Roaminal page, and
   expire the access token. Verify reconnect state recovery, explicit re-open
   after a worker restart, and a useful unavailable error. Browser failure
   must not affect terminal APIs or `/healthz`.
12. Inspect diagnostics and cleanup. No access token, page content, keystroke,
   password, or complete sensitive URL may appear in
   console output, traces, screenshots, worker logs, or server logs. Confirm
   hiding Browser stops frame delivery and that Chromium profile data is
   removed after worker shutdown.

## Pass gate

Fail on any fixture request from the test browser, altered target origin or
path, leaked secret or page content, unbounded frame/input queue, duplicate
input, terminal input while Browser has focus, unexpected WebSocket errors, or
an unguarded Settings exit. A run without the Helm deployment, Pod-only
fixture, or required diagnostics is `BLOCKED`, not passed or skipped.
