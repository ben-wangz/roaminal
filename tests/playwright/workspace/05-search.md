# PW-WORK-005: Terminal search

Priority: P1. Capabilities: core. Viewports: desktop and phone portrait.

## Procedure and assertions

1. Produce terminal output with repeated mixed-case words, whole-word and
   substring matches, and a regex-test pattern. Open Search from the header and
   by Ctrl/Meta+F; the field receives focus without browser Find opening.
2. Search forward with Enter and the Next button; search backward with
   Shift+Enter and Previous. Selection advances/wraps in the active terminal.
3. Toggle case-sensitive, whole-word, and regex independently and in
   combination. Results match xterm search semantics and an invalid regex does
   not throw an uncaught exception.
4. Switch active connection while search is open. Search closes and no query is
   applied to the previous or new terminal without explicit reopening.
5. Close through Escape, the Close button, Connections navigation, and terminal
   exit. Focus returns to a usable workspace control or terminal.
6. Verify searching only reads terminal buffer; it creates no backend search
   endpoint request and persists no query.

## Pass gate

Run the global diagnostics gate. Fail on xterm addon errors, stale selection in
the wrong runtime, or search controls overlapping terminal/top actions.
