# PW-WORK-007: Mobile native input and Virtual Keyboard

Priority: P1. Capabilities: core. Viewports: tablet portrait, phone portrait,
and phone landscape.

## Procedure and assertions

1. Open a live local terminal at phone portrait width. The removed
   TouchKeyboard is absent. Tap the terminal stage and verify the xterm helper
   textarea becomes focused so the browser's native software keyboard opens.
   No `.mobile-terminal-composer`, application-owned input box, or Send control
   is rendered. Dismissing the native keyboard restores the normal chrome.
2. On the affected physical phone and native keyboard, type a non-sensitive
   test word and capture the order of `composition*`, `keydown`/`keyup`,
   `beforeinput`/`input` (including `inputType` and `isComposing`), textarea
   selection/value edit boundaries, and outgoing terminal input frames. Compare
   those frames with terminal echo to distinguish duplicate sends from duplicate
   rendering; redact terminal contents after the comparison. Verify the word
   and native Enter reach the active xterm runtime exactly once. Repeat with a
   commit before and after `compositionend`, including an unpaired post-end
   `insertText` with `isComposing=false`, intervening keydown 229, two deliberate
   identical selections, consecutive compositions, and standalone
   full-width punctuation. Compare punctuation typed immediately after a
   commit and after a pause, then press Enter. Check native sequences that
   replace a visible candidate after `compositionend` separately from edits
   that append punctuation. Verify the commit, punctuation, and Enter reach
   the terminal exactly once and in that order, and no candidate text is sent.
   Repeat after a visual viewport resize and while a focusout event is pending;
   no duplicate frame or input sent to an old runtime is allowed. Browser
   emulation alone cannot establish native IME correctness.
3. Close the native keyboard and open Virtual keyboard from the shared
   icon-only workspace rail. The surface is visible below the terminal and
   contains the Common mode by
   default, including Esc, Tab, Enter, Ctrl+C, Paste, `|`, `~`, `/`, and
   `↑`/`↓`/`←`/`→`. Verify those buttons emit exact terminal sequences, labels
   are fully readable, and the dock does not overlap the terminal or safe-area
   inset. Switch to Tmux and Codex and verify only the selected mode's buttons
   are rendered.
4. When the native keyboard is open again, the Virtual Keyboard key content
   and remote monitor are hidden while the xterm helper remains the only input
   path. The saved Virtual Keyboard preference and monitor disclosure state are
   restored after dismissal.
5. Switch connection, open/close the Connections surface, rotate
   portrait/landscape, and
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
