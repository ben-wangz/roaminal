# PW-WORK-009: Roaminal system status and top-bar layout

Priority: P0. Capabilities: core deployed in a container. Viewports: all five
standard projects.

## Procedure and assertions

1. After heartbeat, the topbar shows Roaminal-scoped CPU/MEM with progress
   semantics, process uptime, optional local RTT, Sign out, and workspace
   actions where applicable. Sessions is available inside Settings, not in the
   topbar. It does not show the active connection
   name, a heartbeat Connected/Reconnecting label, or a connection count.
2. CPU and memory values match the heartbeat payload within formatting rules.
   Capacity/working-set/limit details are truthful for cgroup scope; unavailable
   values show `N/A`/`unlimited` rather than host `free` output or fabricated
   percentages.
3. Change Agent projections across multiple connection instances and verify the
   top-left Connections rail badge follows the number of instances whose Agent
   artwork is `singing-relax`. It must not count non-relaxed Agents, browser
   login sessions, preview sockets, audit history, pending launches, or remote
   monitor probes; the sidebar header and local monitor must not repeat a
   connection total.
4. Stop heartbeat traffic briefly with a controlled route fault. The active
   Terminal footer remains the only visible connection-state owner and is not
   rewritten as a heartbeat state. Successful recovery updates retained local
   metrics without adding a duplicate status label.
5. Exercise a persistence-degraded fixture or mocked heartbeat. The warning is
   visible and accessible without hiding status or terminal controls.
6. At every viewport, the local monitor remains in the top-bar layout flow,
   exposes the `local-monitor` region, and does not overlap the brand or top
   actions. It must not rely on absolute centering. `REMOTE-HEALTH`, when
   present, stays on its own band directly below the topbar without a context
   row.
7. On desktop, the system monitor starts expanded. On tablet and phone, it
   starts collapsed with only the `ROAMINAL` scope label and a real disclosure
   button visible. Toggle it with pointer and keyboard input; verify
   `aria-expanded`, focus behavior, and that collapsed metric content is absent
   from the accessibility tree. Switching connections on a mobile viewport
   returns the system monitor to its collapsed default without changing the
   heartbeat or polling behavior.
8. In Terminal mode, verify the structured terminal footer remains attached
   below the xterm viewport and below the monitor bands. It must not be
   mistaken for local or remote monitor content, and it contains no browser
   clock or footer-specific network request.

## Pass gate

Run the global diagnostics gate. Expected route fault must be narrowly scoped
and restored before final assertions.
