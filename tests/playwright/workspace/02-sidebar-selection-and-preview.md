# PW-WORK-002: Sidebar selection and hover preview

Priority: P0. Capabilities: core with at least three live instances. Viewports:
desktop for preview, tablet/phone for selection.

## Procedure and assertions

1. Verify every card has a title/type, stable shortened ID, accessible lifecycle
   and PWD detail, machine-readable `time[datetime]`, formatted SINCE,
   extension buttons, and an actions menu. There is no terminal/session tab
   strip and no prominent PWD row.
2. Click cards in a non-sequential order. The highlighted card, main viewport
   `data-connection-instance-id`, footer metadata, keyboard mode, and REMOTE
   band all switch to the same instance. The topbar must not become a second
   connection identity owner. Highlight must never move independently of the
   displayed xterm. Assert the footer changes atomically: runtime state,
   connection name, safe endpoint, PWD, TERM, `COLS x ROWS`, and tmux context
   must belong to the selected instance. A reconnecting or exited instance must
   not retain the previous instance's footer values. Verify the Connections
   rail badge follows the current instance total and the sidebar header does not
   repeat it.
3. Verify the sidebar renders the built-in `Ungrouped` section and any user
   groups as unframed sections, while each connection remains an individual
   card. The group header shows its name and member count; a collapsed group
   shows only that structural header and count. The group collapse state is
   restored after reload for the same login session and is independent in a
   second login session.
4. Verify the group header exposes only the collapse and group actions that are
   valid for the current membership. Detailed group creation, persistence,
   ordering, capacity, conflict, and deletion behavior is covered by
   `PW-WORK-017`; this case must not use those mutations as a second owner.
5. Create and retire a disposable connection instance only through the normal
   connection lifecycle. Verify the sidebar reconciles the card without
   changing the selected instance or corrupting the visible group sections.
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
9. In Terminal content, click Agent and verify its status or initialization
   dialog opens without selecting another card. Click the Files action and
   verify it selects the intended connection, opens the Files tool, and keeps
   the right Terminal content until a file is activated; it must not create a
   connection. Activate a file, verify the right content becomes File preview,
   and use `Back to Terminal` to return without changing the active card or
   tree state. The Agent robot remains an Agent details control throughout.
10. On coarse-pointer or width `<=800`, no preview runtime is created by hover,
   focus, or touch. Selecting a card closes the overlay and opens that instance.

## Pass gate

Inspect all preview and main WebSocket events. Fail on leaked preview sockets,
input sent through preview, `onShowLinkUnderline`, disposed runtimes, or any
global diagnostics violation.
