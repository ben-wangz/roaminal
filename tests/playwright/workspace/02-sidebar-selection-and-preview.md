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
3. Verify the sidebar renders the built-in `Ungrouped` section and any user
   groups as unframed sections, while each connection remains an individual
   card. The group header shows its name and member count; a collapsed group
   shows only that structural header and count. The group collapse state is
   restored after reload for the same login session and is independent in a
   second login session.
4. Create a group inline, rename it inline, and verify names are trimmed,
   case-insensitively unique, and persisted after reload. Drag a connection
   within a group, from `Ungrouped` to a group, and back. Drop on a group body
   appends the instance. Drag a group header, including `Ungrouped`, to change
   group order. Repeat the same moves with the keyboard controls. A user group
   accepts at most 10 instances; the 11th move is rejected, leaves the visible
   order unchanged, and reports the capacity reason.
5. Delete an empty group and verify it disappears. A non-empty group's delete
   action is disabled; use `Move all to Ungrouped`, verify all members move in
   their existing order, then delete the now-empty group. Create a new
   connection and verify it always appends to `Ungrouped`, regardless of the
   source definition or the last-used group. An exited instance disappears
   from its group without moving other members.
6. Type a title, ID, PWD, host alias, type, or group name into the sidebar
   search. Matching groups temporarily expand and non-matching cards are
   hidden. Clear the search and verify the previous collapse state returns;
   search does not change the persisted group layout or enable drag mutations.
7. On a fine-pointer desktop, hover a non-active card for longer than the preview
   debounce. A single read-only preview runtime appears in that card while the
   main terminal remains unchanged. Preview does not claim terminal control or
   accept input.
8. Leave the card, close the sidebar, and move rapidly across all cards at least
   100 times. At most one preview exists, stale delayed previews never mount,
   sockets close cleanly, and the main terminal never disappears or auto-cycles.
   Feed a TUI-style stream of cursor movement and redraw sequences while a card
   is previewed. The preview must coalesce output and render at most twice per
   second after its initial snapshot; the changing bottom line must not visibly
   flicker on every WebSocket message. It must replay using the source terminal
   grid, and after the stream settles its final visible line must match the main
   terminal with no duplicated trailing segment or wrapped remainder. The main
   terminal remains real-time.
9. In Terminal mode, click Agent and verify its status or initialization dialog
   opens without selecting another card. In FileSystem mode, click the Agent
   control to return to Terminal; the same control must not lose access to the
   Agent dialog, which remains available from Terminal mode. Click Files and
   verify it enters FileSystem for that card without creating a connection.
10. On coarse-pointer or width `<=800`, no preview runtime is created by hover,
   focus, or touch. Selecting a card closes the overlay and opens that instance.

## Pass gate

Inspect all preview and main WebSocket events. Fail on leaked preview sockets,
input sent through preview, `onShowLinkUnderline`, disposed runtimes, or any
global diagnostics violation.
