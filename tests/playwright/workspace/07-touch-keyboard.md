# PW-WORK-007: Mobile native input and Virtual Keyboard

Priority: P1. Capabilities: core. Viewports: tablet portrait, phone portrait,
and phone landscape.

## Procedure and assertions

1. Open a live local terminal at phone portrait width. The removed legacy
   TouchKeyboard is absent. Tap the terminal stage and verify the xterm helper
   textarea becomes focused so the browser's native software keyboard opens.
   A visible mobile terminal composer stays above it, remains focused, and
   accepts editable text without being covered. Dismissing the native keyboard
   removes the composer.
2. Type a unique draft and tap Send. Capture runtime input frames and verify
   exactly one frame containing the draft followed by one carriage return. The
   draft clears, the composer remains stable, and focus returns to the textarea.
   Repeat after a visual viewport resize, while a focusout event is pending, and
   with an IME composition route. A delayed Enter event never creates an empty
   or duplicate frame; an intentional new draft still sends normally.
3. Close the native keyboard and open the Virtual Keyboard from the topbar.
   The dock is visible above the terminal and contains Common keys Esc, Tab,
   Enter, Ctrl+C, `|`, `~`, `/`, and Up/Down/Left/Right. Verify those buttons
   emit exact terminal sequences and the dock does not overlap the terminal or
   safe-area inset.
4. When the native keyboard is open again, the Virtual Keyboard key content
   and remote monitor are hidden while the composer remains visible. The saved
   Virtual Keyboard preference is restored after dismissal.
5. Switch connection, open/close the sidebar, rotate portrait/landscape, and
   resize the visual viewport. Input always reaches only the active runtime and
   no control shifts terminal geometry unexpectedly.
6. Close/exit the connection and verify all custom input controls are removed
   with the workspace rather than sending into a disposed runtime.

## Pass gate

Run the global diagnostics gate and capture screenshots before/after visual
viewport change. Text, icons, and key labels must remain inside their buttons.
