# PW-WORK-011: Responsive layout and keyboard accessibility

Priority: P0. Capabilities: core, plus SSH/tmux content for longest states.
Viewports: all five standard projects and 320 x 568.

## Procedure and assertions

1. Capture login, Settings, definition editor, SSH keys, Sessions section, local workspace,
   SSH REMOTE workspace, browser Find, action menu, and confirmation
   dialog at applicable viewports. No text/control overlap, clipped buttons,
   incoherent nested scroll, or horizontal page overflow is allowed.
2. At width `<=800`, the Connections and Files surfaces begin closed and inert.
   Opening either from the shared icon rail button shows a backdrop, traps focus,
   closes through backdrop/Escape/Close, and returns focus to the corresponding
   rail button. Connection selection closes the Connections overlay; no hover
   preview is created.
3. Above 800px, open and collapse the shared workspace tool surface from its
   rail/header controls. Main terminal expands and refits when the
   surface is collapsed, and `aria-expanded`, `aria-controls`, `aria-hidden`,
   and inert state remain consistent.
4. In Terminal content, open and collapse Virtual keyboard from the shared rail
   or tool-header control.
   Above 800px it replaces the connection sidebar as a left-side panel; at
   `<=800px` it is below the main Terminal workspace. It never expands the
   connection sidebar at the same time, and its key labels fit at 320px.
   Common, Tmux, and Codex are peer modes with Common selected by default;
   only the selected mode is rendered. Opening either panel closes the other;
   switching to Files changes the left surface and leaves the right Terminal
   content unchanged until a file is activated. Activating a file closes the
   drawer when necessary, opens the right File preview, and preserves the tree
   state when the drawer is reopened.
5. At tablet and phone widths, Files uses the same drawer/backdrop/focus model
   as Connections. Activate a file and verify the right preview uses the
   primary workspace; `Back to Terminal` returns to Terminal without opening
   the native keyboard or resetting the Files tree. Desktop keeps the tree
   visible beside the preview.
6. At tablet and phone widths, both system and remote monitor disclosures start
   collapsed; desktop starts expanded. Each disclosure is an independently
   focusable real button with an accessible name and correct `aria-expanded`
   state, and hidden metrics are absent from the accessibility tree.
   The Terminal footer remains one bounded row: runtime state, connection name,
   PWD, TERM, `COLS x ROWS`, and transport context stay readable, while the
   lower-priority endpoint detail may ellipsize without causing overlap or page
   overflow. The footer has no clock.
7. Navigate every visible command using Tab/Shift+Tab. Focus is visible, order is
   logical, icon-only controls have accessible names, segmented modes expose
   pressed state, and disabled controls are not falsely actionable.
8. For every Modal, focus starts inside, wraps, Escape/backdrop close where
   allowed, and focus returns to a sensible trigger. For action menus verify
   menu keyboard behavior from PW-WORK-003.
9. Verify Ctrl/Meta+Shift+T opens Settings at Connection definitions,
   Ctrl/Meta+F remains available to the browser's native Find UI in every app
   view, and Ctrl/Meta+Shift+S toggles the Connections tool surface.
   Extra Alt or wrong Shift combinations must not trigger them.
10. Emulate `prefers-reduced-motion: reduce`; workspace surface, monitor, and
   preview transitions do not depend on animation for correctness.

## Pass gate

Attach per-viewport screenshots and a focus-order summary. Run the global
diagnostics gate separately for each project; a visually correct screenshot
does not override console, request, or WebSocket failures.
