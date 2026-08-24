# PW-WORK-009: Roaminal system status and top-bar layout

Priority: P0. Capabilities: core deployed in a container. Viewports: all five
standard projects.

## Procedure and assertions

1. After heartbeat, the first row shows Connected, current connection display
   name, Roaminal CPU/MEM with progress semantics, process uptime, connection
   count, heartbeat RTT, Sessions, Sign out, and workspace actions where
   applicable.
2. CPU and memory values match the heartbeat payload within formatting rules.
   Capacity/working-set/limit details are truthful for cgroup scope; unavailable
   values show `N/A`/`unlimited` rather than host `free` output or fabricated
   percentages.
3. Create and close connections and verify `CONN` count follows authoritative
   active instances. It must not count browser login sessions, preview sockets,
   audit history, pending launch after cancellation, or remote monitor probes.
4. Stop heartbeat traffic briefly with a controlled route fault. Header changes
   to Reconnecting without losing the active terminal; successful recovery
   returns Connected and updates RTT.
5. Exercise a persistence-degraded fixture or mocked heartbeat. The warning is
   visible and accessible without hiding status or terminal controls.
6. At every viewport, the local monitor remains on the first row, is visually
   separated/centered relative to `Connected <name>`, and does not overlap the
   connection label or top actions. REMOTE, when present, stays on its own band.
7. On desktop, the system monitor starts expanded. On tablet and phone, it
   starts collapsed with only the connection state/name and a real disclosure
   button visible. Toggle it with pointer and keyboard input; verify
   `aria-expanded`, focus behavior, and that collapsed metric content is absent
   from the accessibility tree. Switching connections on a mobile viewport
   returns the system monitor to its collapsed default without changing the
   heartbeat or polling behavior.

## Pass gate

Run the global diagnostics gate. Expected route fault must be narrowly scoped
and restored before final assertions.
