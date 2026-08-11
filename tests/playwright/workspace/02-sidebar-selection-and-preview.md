# PW-WORK-002: Sidebar selection and hover preview

Priority: P0. Capabilities: core with at least three live instances. Viewports:
desktop for preview, tablet/phone for selection.

## Procedure and assertions

1. Verify every card has a title/state, stable shortened ID, optional PWD/TARGET,
   machine-readable `time[datetime]`, formatted SINCE, extension buttons, and an
   actions menu. There is no terminal/session tab strip.
2. Click cards in a non-sequential order. The highlighted card, top connection
   name, main viewport `data-connection-instance-id`, footer metadata, keyboard
   mode, and REMOTE band all switch to the same instance. Highlight must never
   move independently of the displayed xterm.
3. On a fine-pointer desktop, hover a non-active card for longer than the preview
   debounce. A single read-only preview runtime appears in that card while the
   main terminal remains unchanged. Preview does not claim terminal control or
   accept input.
4. Leave the card, close the sidebar, and move rapidly across all cards at least
   100 times. At most one preview exists, stale delayed previews never mount,
   sockets close cleanly, and the main terminal never disappears or auto-cycles.
5. Click Agent and Files extension controls. Each reports its unavailable toast,
   does not select another card, and does not navigate or create a connection.
6. On coarse-pointer or width `<=800`, no preview runtime is created by hover,
   focus, or touch. Selecting a card closes the overlay and opens that instance.

## Pass gate

Inspect all preview and main WebSocket events. Fail on leaked preview sockets,
input sent through preview, `onShowLinkUnderline`, disposed runtimes, or any
global diagnostics violation.
