# PW-WORK-008: Terminal resize and geometry stability

Priority: P0. Capabilities: core; repeat on tmux when available. Viewports: all
five standard projects plus dynamic resizing.

## Procedure and assertions

1. Open a local connection and compare xterm columns/rows with the footer's
   `COLS x ROWS` field, sidebar metadata, and the heartbeat resize update.
   Verify `TERM` remains present and values stay in supported bounds.
2. Resize the desktop viewport through narrow/wide and short/tall dimensions,
   collapse/open the shared workspace tool surface, switch between Connections
   and Virtual keyboard, use browser Find, and show/hide the remote monitor.
   After each stable layout, xterm fits once to available space and sends the
   converged geometry without an endless resize loop.
3. Run `stty size` after each state. Reported PTY rows/columns match xterm and
   the footer's combined grid field; footer updates do not issue an independent
   resize request.
4. Rapidly resize at least 30 times, then type a marker. The terminal remains
   interactive, one xterm stays mounted, and cursor/output are visible.
5. Repeat through SSH and tmux. The remote PTY and active tmux client adapt to
   the current screen; reconnect/attach must not restore a stale larger size.
6. Verify hover preview fixed geometry cannot resize the main PTY.

## Pass gate

Fail on text overlap, blank canvas/xterm, page horizontal overflow, continuous
heartbeat resize traffic after settling, wrong tmux size, or any global
diagnostics violation.
