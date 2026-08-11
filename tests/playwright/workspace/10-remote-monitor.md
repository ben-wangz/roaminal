# PW-WORK-010: Remote SSH monitor

Priority: P0. Capabilities: SSH fixture with observable `/proc`, cgroup, and
rootfs; additional controlled response fixtures for degraded states. Viewports:
desktop, tablet portrait, and phone portrait.

## Procedure and assertions

1. Local connection and Connection manager show no REMOTE band and issue no
   remote-monitor requests. Selecting a live SSH instance adds a separate
   `REMOTE <host-alias>` band below the top row and polls only that instance.
2. Observe warming then a later sample. Validate CPU, MEM, PID1 uptime, SYSTEM
   load 1/5/15, ROOTFS disk used/total/percent, freshness AGE, and probe RTT
   against the endpoint payload and fixture tolerances.
3. Verify CPU/MEM scope displays `CGROUP 1`, `CGROUP 2`, `HOST`, or unavailable
   exactly as returned. Uptime, load, and disk retain their independent PID1,
   SYSTEM, and ROOTFS labels; no unproven Pod/Container label appears.
4. Exercise available, partial, stale, and unavailable responses. Nullable
   metrics show N/A while reliable metrics remain. The previous sample's age is
   explicit; stale values never masquerade as fresh.
5. Make a probe fail, exceed its timeout, and recover. Terminal input, SSH
   WebSocket, tmux resize, local Roaminal monitor, and other connections remain
   usable throughout; the band reports failure and resumes polling.
6. Switch quickly between two transports and local. Abort/ignore late responses
   from the old ID, never flash old metrics under the new alias, and stop
   requests for inactive/local/exited instances and manager view.
7. Put the page in a true background/hidden state, verify polling pauses, then
   restore visibility and verify one immediate poll resumes without a burst.
8. In two browser contexts and multiple derived instances on one transport,
   use fixture-side probe counters to confirm server cache/singleflight prevents
   probe count from scaling linearly.

## Pass gate

Expected monitor failures are allowed only for their exact request. Raw remote
collector output must never enter DOM, browser storage, logs, or reports. Run
the global diagnostics gate and ensure polling cleanup completes before close.
