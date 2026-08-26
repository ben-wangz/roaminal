# PW-WORK-007: Mobile native input and Virtual Keyboard

Priority: P1. Capabilities: core. Viewports: tablet portrait, phone portrait,
and phone landscape.

## Procedure and assertions

1. Open a live local terminal at phone portrait width. The removed legacy
   TouchKeyboard is absent. Tap the terminal stage and verify the xterm helper
   textarea becomes focused so the browser's native software keyboard opens.
   No `.mobile-terminal-composer`, application-owned input box, or Send control
   is rendered. Dismissing the native keyboard restores the normal chrome.
2. Type unique text through the native keyboard and capture runtime input
   frames. Verify text and native Enter reach the active xterm runtime exactly
   once. Repeat with Chinese IME composition and full-width punctuation; the
   existing IME fallback handles committed text without a mobile-only rewrite.
   Repeat after a visual viewport resize and while a focusout event is pending;
   no duplicate frame or input sent to an old runtime is allowed.
3. Close the native keyboard and open the Virtual Keyboard from the topbar.
   The dock is visible above the terminal and contains Common keys Esc, Tab,
   Enter, Ctrl+C, `|`, `~`, `/`, and `↑`/`↓`/`←`/`→`. Verify those buttons emit
   exact terminal sequences, every key is at most 12px tall, and the dock does
   not overlap the terminal or safe-area inset.
4. When the native keyboard is open again, the Virtual Keyboard key content
   and remote monitor are hidden while the xterm helper remains the only input
   path. The saved Virtual Keyboard preference and monitor disclosure state are
   restored after dismissal.
5. Switch connection, open/close the sidebar, rotate portrait/landscape, and
   resize the visual viewport. Verify the entire Roaminal frame ends above the
   emulated keyboard in both content-resize and overlay/visual-viewport
   geometry cases. Terminal fitting follows each viewport transition once; no
   empty composer gap, covered terminal content, or horizontal page overflow is
   present. Input always reaches only the active runtime.
6. Close/exit the connection and verify all custom input controls are removed
   with the workspace rather than sending into a disposed runtime.

## Pass gate

Run the global diagnostics gate and capture screenshots before/after visual
viewport change. Text, icons, and key labels must remain inside their buttons.
