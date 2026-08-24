# PW-WORK-011: Responsive layout and keyboard accessibility

Priority: P0. Capabilities: core, plus SSH/tmux content for longest states.
Viewports: all five standard projects and 320 x 568.

## Procedure and assertions

1. Capture login, connection manager, definition editor, Keys, local workspace,
   SSH REMOTE workspace, search, Sessions dialog, action menu, and confirmation
   dialog at applicable viewports. No text/control overlap, clipped buttons,
   incoherent nested scroll, or horizontal page overflow is allowed.
2. At width `<=800`, sidebar begins closed and inert. Open sidebar has a backdrop,
   traps focus, closes through backdrop/Escape/Close, and returns focus to Open
   sidebar. Selection closes the overlay; no hover preview is created.
3. Above 800px, toggle sidebar from its real icon control. Main terminal expands
   and refits, the Open sidebar button restores it, and `aria-expanded`,
   `aria-controls`, `aria-hidden`, and inert state remain consistent.
4. In Terminal mode, open and collapse the Virtual Keyboard from the topbar.
   Its bottom dock stays inside the main panel, never expands the connection
   sidebar at the same time, and its key labels fit at 320px. Opening either
   panel closes the other; switching to FileSystem hides the dock.
5. At tablet and phone widths, both system and remote monitor disclosures start
   collapsed; desktop starts expanded. Each disclosure is an independently
   focusable real button with an accessible name and correct `aria-expanded`
   state, and hidden metrics are absent from the accessibility tree.
6. Navigate every visible command using Tab/Shift+Tab. Focus is visible, order is
   logical, icon-only controls have accessible names, segmented modes expose
   pressed state, and disabled controls are not falsely actionable.
7. For every Modal, focus starts inside, wraps, Escape/backdrop close where
   allowed, and focus returns to a sensible trigger. For action menus verify
   menu keyboard behavior from PW-WORK-003.
8. Verify Ctrl/Meta+Shift+T opens Connection manager, Ctrl/Meta+F opens terminal
   search only with an active connection, and Ctrl/Meta+Shift+S toggles sidebar.
   Extra Alt or wrong Shift combinations must not trigger them.
9. Emulate `prefers-reduced-motion: reduce`; sidebar, monitor, and preview transitions do
   not depend on animation for correctness.

## Pass gate

Attach per-viewport screenshots and a focus-order summary. Run the global
diagnostics gate separately for each project; a visually correct screenshot
does not override console, request, or WebSocket failures.
