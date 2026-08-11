# PW-REL-005: Connection/client capacity and state isolation

Priority: P1. Capabilities: dedicated release configured with small connection
and client limits. Viewport: desktop. Run serially.

## Procedure and assertions

1. Configure `maxConnectionInstances=2`. Create two live connections and attempt
   a third local, SSH, and pending tmux launch in separate resets. Each excess
   attempt gets the documented conflict, shows one toast, creates no process,
   metadata, card, or orphan launch, and existing terminals stay usable.
2. Close one connection and immediately create another. Capacity is released
   exactly once with no race or negative/leaked reservation.
3. Configure a small client limit. Attach the allowed number of browser main and
   preview clients to one instance, then attempt one more. Only the excess
   attach gets `429 client capacity reached`; existing clients remain connected.
4. Dispose a preview/browser and retry. The released slot is reusable.
5. Create same-looking aliases/titles across local and SSH instances and produce
   distinct markers. Heartbeat, snapshots, PWD, title, monitor cache, transport
   reuse, and browser active-selection state remain keyed by the exact instance
   and never bleed across them.
6. Run two simultaneous create requests for the final slot. At most one succeeds
   because reservations are atomic.

## Cleanup and pass gate

Close every created instance and restore normal limits through a release reset.
Expected capacity responses are the only failures; run the global diagnostics
gate in all contexts.
