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
4. Verify the toolbar exposes Back, Forward, Reload, page identity, Open
   address, and an icon-only Back to terminal action. The page identity shows
   `Network: Roaminal`. Open address must be a dialog, not a permanent browser
   address bar. Cancel leaves the current page unchanged.
5. Try invalid, credential-bearing, `file:`, and browser-internal addresses.
   Verify each is rejected without a target request.
6. Exercise pointer click, drag, wheel, keyboard input, password input,
   Chinese IME text, and viewport resize. Verify input reaches the fixture
   page, no terminal receives browser keystrokes, the canvas remains within
   the workspace, and no local file picker or download is opened.
7. Switch repeatedly between Browser, Terminal, Connections, and Files while
   a terminal command is running. Browser page state and terminal process
   state remain intact. Settings preserves its unsaved-changes guard and
   returns to Browser even when there are no terminal connections.
8. Close the browser WebSocket, refresh the Roaminal page, and expire the
   access token. Verify reconnect state recovery, explicit re-open after a
   worker restart, one-viewer conflict behavior, and a useful unavailable
   error. Browser failure must not affect terminal APIs or `/healthz`.
9. Inspect diagnostics and cleanup. No access token, page content, keystroke,
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
