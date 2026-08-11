# PW-REL-001: Browser refresh and active-workspace recovery

Priority: P0. Capabilities: core; repeat for SSH/tmux when available. Viewport:
desktop.

## Procedure and assertions

1. Create two live connections, select the second, produce unique scrollback and
   change its PWD/title. Wait for heartbeat and the debounced snapshot interval.
2. Reload the page. The stored active connection instance is selected, exactly
   one main xterm attaches, and snapshot/meta/status arrive before live output.
   The process did not restart and a post-refresh command sees the same shell
   state where the underlying connection type supports it.
3. Scrollback, title, PWD, dimensions, and sidebar order recover without
   duplicated prompt/output. Old legacy local-storage keys for terminal tabs are
   removed and no tab UI returns.
4. Reload rapidly five times, including once while a preview is connecting.
   Main/preview runtimes dispose cleanly, there is no early-open socket warning,
   and final input reaches the selected instance once.
5. Delete/exit the stored active instance from another browser context, then
   reload. Selection reconciles to the next surviving instance; if none remains,
   the manager opens.
6. Put malformed JSON in only the active-selection local-storage key and reload.
   The app recovers to a safe selection/manager rather than crashing or losing
   authentication.

## Pass gate

Fail on `terminal runtime ... is disposed`, WebSocket-close-before-established,
duplicate xterms, duplicate output, stale selection, or any global diagnostics
violation.
