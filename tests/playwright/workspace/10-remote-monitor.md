# PW-WORK-010: Remote SSH monitor

Priority: P0. Capabilities: SSH fixture with observable `/proc`, cgroup, and
rootfs; additional controlled response fixtures for degraded states. Viewports:
desktop, tablet portrait, and phone portrait.

## Procedure and assertions

1. Local connection and Connection manager show no REMOTE band and issue no
   remote-monitor requests. Selecting a live SSH instance adds a separate
   `REMOTE <host-alias>` band below the top row and polls only that instance.
   It is expanded by default on desktop and collapsed by default on tablet and
   phone; the collapsed band still exposes REMOTE, host alias, status, and a
   keyboard-reachable disclosure button.
2. Observe warming then a later sample. Validate the header status and the
   separate primary-resource group for CPU, MEM, and DISK. Each primary metric
   has a readable value, complete detail, a semantic progressbar when a
   percentage exists, and CPU/MEM trend lines after two samples. Validate PID1
   uptime, SYSTEM load 1/5/15, ROOTFS disk used/total/percent, freshness AGE,
   and probe RTT against the endpoint payload and fixture tolerances.
3. Verify CPU/MEM scope displays `CGROUP 1`, `CGROUP 2`, `HOST`, or unavailable
   exactly as returned. Uptime, load, and disk retain their independent PID1,
   SYSTEM, and ROOTFS labels; no unproven Pod/Container label appears.
4. Exercise available, partial, stale, and unavailable responses. The header
   combines backend status with client request state: warming before the first
   response, unavailable after a failed request without a sample, and stale
   with `probe unavailable` when a cached sample exists. Nullable metrics show
   N/A while reliable metrics remain. The previous sample's age is explicit;
   stale values never masquerade as fresh.
5. Make a probe fail, exceed its timeout, and recover. Terminal input, SSH
   WebSocket, tmux resize, local Roaminal monitor, and other connections remain
   usable throughout; the band reports failure and resumes polling. Hold one
   monitor HTTP response open, then verify a later poll is issued after the
   client timeout and a successful sample clears `probe unavailable`.
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

## Pass gate

Expected monitor failures are allowed only for their exact request. Raw remote
collector output must never enter DOM, browser storage, logs, or reports. Run
the global diagnostics gate and ensure polling cleanup completes before close.
