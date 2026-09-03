# PW-WORK-019: Main workspace shell and tool rail

Priority: P1. Capabilities: core. Viewports: desktop, tablet landscape, tablet
portrait, phone portrait, and phone landscape.

## Procedure and assertions

1. Run the mandatory Helm deployment and browser diagnostics gates from the
   Playwright README. Authenticate through the visible login flow and select a
   live connection instance. Record the release, version, boot ID, browser,
   viewport, and diagnostic result.
2. At desktop width verify the authenticated workspace has one full-width
   topbar, followed by one workspace row containing the icon-only tool rail,
   the selected tool surface, and the main panel. Verify the rail and tool
   surface start below the topbar and extend to the bottom of the viewport.
   Verify there is no document-level horizontal or vertical overflow.
3. Verify the topbar still exposes the brand, Roaminal-scoped resource metrics,
   Messages, Fullscreen, and Sign out. Verify Sessions is available from the
   Settings page rather than the topbar, and Ctrl/Meta+F is left to browser Find.
   Verify it
   does not expose a Connections or Virtual keyboard workspace switcher, active
   connection identity, heartbeat state, or a connection count. The
   `fullscreen-toggle` remains present and retains its runtime-driven supported,
   pending, active, or unsupported state.
4. Verify the rail has exactly one Connections control, one Virtual keyboard
   control, one Files control, one Settings control, one Help control, and one
   collapse control. Use
   accessible names and `data-testid` values rather than icon or CSS selectors.
   The active tool has an active background and `aria-pressed`; Virtual
   keyboard remains selectable while File preview is visible and returns the
   right content to Terminal. Click Help and verify the concise toast
   `User manual is being prepared.` appears without navigation or network
   activity.
5. Click Connections and verify it opens or collapses the same tool surface.
   When open, verify the panel has a header without a repeated count,
   connection search, grouped connection-instance cards, and an `Add connection`
   action at the bottom. Verify the rail `PanelLeft` control owns the numeric
   top-left Agent relax count badge.
   Verify panel scrolling is independent from the monitor and terminal and the
   terminal remains visible in the main panel.
6. Click `Add connection` and verify a modal opens without leaving the
   workspace. Verify the labeled definition dropdown includes `Local` and the
   available SSH connection definitions, has an empty initial selection, and
   Confirm is disabled until a selection is made. Cancel and Escape close the
   modal without creating an instance. Open it again, select a controlled
   definition, confirm exactly once, and correlate the existing connection
   create/launch request. Verify duplicate clicks are prevented; on success the
   dialog closes and the new instance becomes active. Force a controlled create
   failure and verify the modal stays open, exposes an actionable error, and
   permits one retry.
7. Verify each connection card retains its current title/type, lifecycle
   indicator, ID, accessible PWD/CWD detail, SINCE, pointer preview behavior, selection, Terminal
   action, FileSystem action where available, agent artwork/action, action menu,
   group actions, and drag/keyboard reorder behavior. Do not infer action
   identity from the reference image. Selecting a card or Terminal action must
   leave only one active terminal runtime. The Files action selects the card,
   opens the Files tool in the left surface, and keeps Terminal visible until a
   file is activated; it must not append a second workspace or create a
   connection.
8. Verify there is no selected-connection context row above the remote
   monitor. A live SSH instance renders one compact `REMOTE-HEALTH` band with
   one disclosure control; expanding/collapsing it changes only the monitor
   metrics and never changes the selected connection. Its header has no host,
   connection name, or visible Available/Stale/Unavailable/probe text. Verify
   the expanded row exposes CPU, MEM, DISK, UPTIME, LOAD, AGE, and RTT in that
   order. The terminal itself ends with one compact structured footer containing
   runtime state, connection name, safe endpoint, PWD, TERM, `COLS x ROWS`, and
   tmux/ssh/local context. It has no clock, remains attached below xterm, and
   reduces predictably on narrow screens.
9. Open Virtual keyboard from the rail and verify the currently selected
   Connections or Files surface collapses before
   the keyboard surface opens. Verify Common, Tmux, and Codex modes and all
   existing key actions remain available. Return to Connections and verify no
   duplicate tool surface or terminal runtime appears. Collapse the active tool
   and verify focus returns to its rail control; the collapsed surface is not
   in the tab order.
10. At tablet and phone widths verify the same rail controls remain usable
    without forcing the desktop vertical geometry. Connections keeps its
    existing overlay/backdrop behavior, Virtual keyboard keeps its existing
    below-terminal placement, native keyboard handling, safe-area behavior, and
    focus rules, and no old topbar tool buttons reappear. The Help and Add
    connection controls remain reachable with touch and keyboard.
11. Switch the left rail between Connections and Files, activate a text/Markdown
    file, use `Back to Terminal`, use browser Find, trigger a message-center
    navigation, and exercise the
    fullscreen control. Verify the shell transition does not reset terminal
    scrollback, Markdown preview position, message state, connection selection,
    browser notification state, or agent state. Fullscreen remains a browser
    capability, not a workspace-tool action.
12. Finish with the global diagnostics gate. Fail on React errors, console
    warnings/errors, request failures, unexpected 4xx/5xx responses,
    `ERR_INSUFFICIENT_RESOURCES`, duplicate `/notifications/preferences` or
    other stable-resource requests, duplicate terminal output, hidden focusable
    tool surfaces, overflow, or any mutation not caused by an explicit user
    action. Clean up only instances and definitions created by this case.

## Pass gate

Capture final desktop, tablet, and phone screenshots showing the rail, open
Connections surface, Add connection modal, Virtual keyboard surface, monitor,
    footer, and Terminal/File preview replacement. Record all API
correlations and diagnostics. The case passes only when the visual hierarchy
and the preserved behavior both satisfy the assertions.
