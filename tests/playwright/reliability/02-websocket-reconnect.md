# PW-REL-002: Terminal WebSocket interruption and reconnect

Priority: P0. Capabilities: core plus a controllable test proxy/network rule.
Viewport: desktop. Run serially.

## Procedure and assertions

1. Open a local connection and establish a marker baseline. Interrupt only its
   WebSocket while keeping HTTP heartbeat reachable.
2. Header/terminal state reflects disconnection appropriately, contextual keys
   are disabled, and the terminal runtime schedules bounded reconnect attempts.
   It does not create another connection instance or dispose the xterm.
3. Restore the path before the next retry. A new socket authenticates with the
   current token, receives one snapshot plus current meta/status, and terminal
   input/output resume without duplicates.
4. Repeat across access-token rotation. The reconnect must resolve the newly
   stored token, not reuse the token captured at initial render.
5. Interrupt while switching between connections and while a preview is open.
   Only the active main runtime reconnects persistently; preview disposal and
   stale runtime timers cannot reattach to the DOM.
6. Terminate the underlying connection during outage. On restoration, no
   endless reconnect loop targets the retired instance and the UI follows normal
   failover.

## Pass gate

The intentionally interrupted request/socket is the only allowed network
failure. Fail on HTTP-authentication console messages, disposed-runtime errors,
an unbounded socket storm, or any other global diagnostics violation.
