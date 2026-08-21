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
3. On desktop, drag the Reorder connection grip on the last card before the
   first card. The visible order updates immediately and remains the same after
   reload. A newly created instance appends after the saved order; an exited
   instance is ignored. In a second login session, its sidebar order remains
   independent.
4. On a fine-pointer desktop, hover a non-active card for longer than the preview
   debounce. A single read-only preview runtime appears in that card while the
   main terminal remains unchanged. Preview does not claim terminal control or
   accept input.
5. Leave the card, close the sidebar, and move rapidly across all cards at least
   100 times. At most one preview exists, stale delayed previews never mount,
   sockets close cleanly, and the main terminal never disappears or auto-cycles.
   Feed a TUI-style stream of cursor movement and redraw sequences while a card
   is previewed. The preview must coalesce output and render at most twice per
   second after its initial snapshot; the changing bottom line must not visibly
   flicker on every WebSocket message. It must replay using the source terminal
   grid, and after the stream settles its final visible line must match the main
   terminal with no duplicated trailing segment or wrapped remainder. The main
   terminal remains real-time.
6. Click Agent and Files extension controls. Agent opens its status or
   initialization dialog without selecting another card; Files preserves its
   existing behavior. Neither control navigates or creates a connection.
7. On coarse-pointer or width `<=800`, no preview runtime is created by hover,
   focus, or touch. Selecting a card closes the overlay and opens that instance.

## Pass gate

Inspect all preview and main WebSocket events. Fail on leaked preview sockets,
input sent through preview, `onShowLinkUnderline`, disposed runtimes, or any
global diagnostics violation.
