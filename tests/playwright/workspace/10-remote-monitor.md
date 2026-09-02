# PW-WORK-010: Remote SSH monitor

Priority: P0. Capabilities: SSH fixture with observable `/proc`, cgroup, and
rootfs; additional controlled response fixtures for degraded states. Viewports:
desktop, tablet portrait, and phone portrait.

## Procedure and assertions

1. Local connection and Settings show no Remote monitor band and issue no
   remote-monitor requests. Selecting a live SSH instance adds one
   `REMOTE-HEALTH` band directly below the topbar and polls only that instance.
   There is no selected-connection context row. The remote band owns exactly
   one disclosure control (`remote-monitor-toggle`) in its header. It is
   expanded by default on desktop and collapsed by default on tablet and phone;
   the collapsed band exposes the `REMOTE-HEALTH` label, a color state, an
   accessible health name, and a keyboard-reachable disclosure button.
2. Observe warming then a later sample. Validate the header health color and
   separate primary-resource group for CPU, MEM, and DISK. Each primary metric
   has a readable value, complete detail, a semantic progressbar when a
   percentage exists, and CPU/MEM trend lines after two samples. Validate the
   stable metric order CPU, MEM, DISK, UPTIME, LOAD, AGE, RTT; validate PID1
   uptime, SYSTEM load 1/5/15, ROOTFS disk used/total/percent, freshness AGE,
   and probe RTT against the endpoint payload and fixture tolerances.
3. Verify CPU/MEM scope displays `CGROUP 1`, `CGROUP 2`, `HOST`, or unavailable
   exactly as returned. Uptime, load, and disk retain their independent PID1,
   SYSTEM, and ROOTFS labels; no unproven Pod/Container label appears.
4. Exercise available, partial, stale, and unavailable responses. The header
   maps available to green, warming/partial/stale to the warning color, and
   unavailable to the error color. Its semantic accessible name exposes
   Available, Stale, or Unavailable, but no visible status or probe-failure
   sentence is rendered. Nullable metrics show N/A while reliable metrics
   remain. The previous sample's age is explicit; stale values never masquerade
   as fresh.
5. Make a probe fail, exceed its timeout, and recover. Terminal input, SSH
   WebSocket, tmux resize, local Roaminal monitor, and other connections remain
   usable throughout; the band reports failure and resumes polling. Hold one
   monitor HTTP response open, then verify a later poll is issued after the
   client timeout and a successful sample returns the header to the healthy
   color.
6. Switch quickly between two transports and local. Abort/ignore late responses
   from the old ID, never flash old metrics under the new alias, and stop
   requests for inactive/local/exited instances and manager view.
7. Put the page in a true background/hidden state, verify polling pauses, then
   restore visibility and verify one immediate poll resumes without a burst.
8. In two browser contexts and multiple derived instances on one transport,
   use fixture-side probe counters to confirm server cache/singleflight prevents
   probe count from scaling linearly. Also start at least six independent live
   SSH transports (repeat with tmux-enabled and plain SSH definitions) and
   request their monitor endpoints concurrently. Every request must return
   `200` with `warming`, `partial`, or `available`; a full probe pool must
   queue work and must not make later connections report `status: unavailable`
   solely because they arrived after the first four.
9. Expand and collapse the remote monitor independently of the system monitor.
   Verify collapsed content is conditionally absent from the accessibility tree,
   the last sample remains available, polling continues without an extra burst,
   and switching to another SSH instance applies that instance's mobile
   collapsed default.

10. In Terminal mode, verify the footer stays below the terminal when
    `REMOTE-HEALTH` expands or collapses. Its runtime state, connection name,
    safe endpoint, PWD, TERM, `COLS x ROWS`, and tmux/ssh/local context remain
    in the footer rather than being merged into the health band; monitor
    polling must not be duplicated for the footer.

## Pass gate

Expected monitor failures are allowed only for their exact request. Raw remote
collector output must never enter DOM, browser storage, logs, or reports. Run
the global diagnostics gate and ensure polling cleanup completes before close.
