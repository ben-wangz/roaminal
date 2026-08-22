# PW-WORK-007: Mobile touch keyboard

Priority: P1. Capabilities: core. Viewports: tablet portrait, phone portrait,
and phone landscape.

## Procedure and assertions

1. Open a live local terminal. The touch keyboard is visible only at width
   `<=800`; it does not overlap xterm, status bar, or browser-safe viewport
   height when the software keyboard changes visual viewport size. Tap the
   terminal stage and verify the xterm helper textarea becomes the focused
   element so the browser's native software keyboard opens for command input.
   When the native keyboard is open, a visible mobile terminal composer stays
   above it, remains focused, and accepts editable text without being covered.
   Pressing Enter or the send button sends the draft once with a trailing
   carriage return; dismissing the native keyboard removes the composer.
2. Through a byte-capture command, verify ESC, TAB, and arrow buttons emit exact
   terminal sequences.
3. Toggle SHIFT, CTRL, ALT, and SYM. `aria-pressed` and visual state follow the
   toggle. SHIFT uppercases a single character, CTRL maps one ASCII letter to
   its control byte, and ALT prefixes Escape.
4. After a non-modifier key, all active modifiers reset. Toggling a modifier off
   before a key sends the unmodified value.
5. Switch connection, open/close the sidebar, rotate portrait/landscape, and
   resize the visual viewport. Input always reaches only the active runtime and
   no control shifts terminal geometry unexpectedly.
6. Close/exit the connection and verify the keyboard is removed with the
   workspace rather than sending into a disposed runtime.

## Pass gate

Run the global diagnostics gate and capture screenshots before/after visual
viewport change. Text, icons, and key labels must remain inside their buttons.
