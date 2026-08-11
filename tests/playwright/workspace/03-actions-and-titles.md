# PW-WORK-003: Connection action menu, rename, and close

Priority: P0. Capabilities: core. Viewports: desktop and phone portrait.

## Procedure and assertions

1. Open a live card's Terminal actions. The first menu item is focused; menu
   semantics, `aria-expanded`, outside-click close, Escape focus restoration,
   ArrowUp/ArrowDown/Home/End wrapping, and viewport-safe menu placement work.
2. The live menu contains `Rename title...` and `Close connection...`; it never
   contains a tab-close command. Select Rename.
3. The modal traps focus. Empty/whitespace, more than 128 Unicode code points or
   512 UTF-8 bytes, control characters, and bidi-control characters are rejected
   without a PATCH. A valid trimmed title saves, appears on the card/document
   title, and persists through manager navigation and reload.
4. Reopen the menu. `Use automatic title` appears only for a custom title.
   Activate it and verify shell-provided/automatic title behavior resumes and
   survives reload.
5. Choose Close, cancel once, then confirm. The dialog identifies the intended
   instance, disables while working, archives/retires it, and selects the next
   connection or manager according to the failover rule.
6. Ensure actions on a background card affect that card only and do not send
   input to or unexpectedly select it before the requested mutation.

## Pass gate

Run the global diagnostics gate. A close must remove the active instance rather
than leave an exited history card; the current workflow has no history action.
