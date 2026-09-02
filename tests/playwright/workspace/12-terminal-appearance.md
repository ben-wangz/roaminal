# PW-WORK-012: Terminal appearance preferences

Priority: P1. Capabilities: core; repeat with SSH/tmux when fixtures are
available. Viewports: all five standard projects; use desktop for lifecycle and
cross-tab assertions.

## Preconditions

1. Complete the mandatory Helm deployment gate and browser diagnostics gate in
   [`tests/playwright/README.md`](../README.md). If the release is missing or
   not Ready, follow [the Helm deployment procedure](../../../docs/deployment.md#kubernetes-with-helm)
   and rerun the gate before opening a browser.
2. Start with a fresh browser profile and authenticate through the visible
   login UI. Keep a dedicated second context for cross-tab storage behavior.
3. Record the initial `bootId`, connection instance IDs, WebSocket URLs, and
   console/request diagnostics. Treat every error, warning, uncaught exception,
   unexpected failed request, and unexpected WebSocket close as a failure.

## Cases

1. Open Settings from the application tool rail and select `Interface`. Verify
   it is a first-class section, not a modal or workspace tool tab. The section
   exposes a font selector, synchronized font-size range/number controls, live
   read-only xterm sample, Save, and Reset to defaults. Switch to another
   Settings section and back without losing an unsaved draft.
2. In a fresh profile, assert defaults are `Monaspace Neon` and `12px`. Select
   each bundled font and assert the sample changes family, the browser reports
   `document.fonts.check(...)`, and no font request leaves the Roaminal origin.
   Select System Monospace and verify it remains usable without a remote font
   request.
3. Set the range to 22 and assert the number input is 22; set the number input
   to 11 and assert the range is 11. Try 9, 33, decimals, text, and an empty
   value. Save stays disabled and an accessible validation message appears
   until the value is an integer from 10 through 32. Reset changes the draft
   and sample only; it does not change local storage until Save.
4. Navigate away from Settings with an unsaved draft, cancel the discard
   confirmation once, then confirm it and return. The draft is discarded.
   Save a non-default font and size, reload, sign out, sign back in, and assert
   the saved preference remains. Create a separate fresh browser context and
   assert it still starts with the defaults.
5. Create one local connection and capture its instance ID, xterm element, and
   connection WebSocket. Change the font and size, then assert the same
   connection instance, xterm runtime, and WebSocket remain; output remains
   interactive; only the expected resize messages occur after the font is
   loaded; and `stty size` converges to the visible xterm grid. There must be
   no duplicate xterm, duplicate socket, lost output, or terminal disposal
   error. Apply several values quickly and verify the final value wins without
   continuous resize traffic.
6. Open a second authenticated tab/context, change and save the preference
   there, and assert the active terminal in the first tab adopts it once. Close
   the second context and verify no additional socket or resize loop remains.
   Remove the storage key in the second context and assert the first tab safely
   returns to defaults.
7. Hover a sidebar connection to mount its preview. Assert its font family
   matches the saved family while its xterm font size remains fixed at 10px,
   card dimensions remain stable, and only one preview WebSocket exists. Feed
   high-frequency TUI redraw output and retain the existing preview coalescing
   behavior; the bottom line must not visibly repaint on every message.
8. With a tmux-enabled SSH fixture, record the remote `~/.tmux.conf` checksum,
   `tmux show-options -gv window-size`, the connection instance ID, and the
   WebSocket. Change font and size repeatedly. Assert Roaminal changes only
   browser xterm metrics and PTY resize messages: the tmux config and window
   policy are unchanged, the effective tmux grid remains usable, colors/status
   content remain intact, and no resize loop or reconnect occurs.
9. Before reload, inject malformed JSON, an unknown font ID, schema version 2,
   a fractional size, 9, and 33 into `localStorage['roaminal.appearance.v2']`
   in separate runs. The UI must restore defaults without a page error, console
   warning, failed request, blank sample, or external font request.
10. Repeat the page, controls, sample, and 32px setting at desktop, tablet
   landscape, tablet portrait, phone portrait, and phone landscape. Assert no
   clipped text, overlapping controls, horizontal page overflow, blank xterm,
   or inaccessible control. Verify screenshots at the saved final state.
11. While Settings > Interface is mounted, record the sample xterm element,
    runtime identity, rendered grid, and WebSocket count. Trigger at least 30
    heartbeat cycles, switch connection selection, open/close Message Center,
    toggle notification state, and open/close unrelated dialogs. Assert the
    sample keeps one runtime and stable rendered content/grid; only a font or
    size draft change may change its metrics. No duplicate socket, disposed
    runtime, blank sample, or continuous redraw/request loop is allowed.

## Cleanup and diagnostics

Restore the saved appearance key, remove created connection instances, close
the second context, and leave the Helm release and pre-existing SSH/tmux data
unchanged. Before reporting `PASS`, inspect every captured console message,
page error, failed request, response, and WebSocket event. In particular fail
on `onShowLinkUnderline`, `terminal runtime ... is disposed`, `WebSocket is
closed before the connection is established`, `HTTP Authentication failed; no
valid credentials available`, unexpected `invalid session id`, `ssh transport
unavailable`, or `ssh transport is draining`.
