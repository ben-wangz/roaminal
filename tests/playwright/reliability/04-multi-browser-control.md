# PW-REL-004: Multiple browsers viewing one connection

Priority: P1. Capabilities: core and two authenticated browser contexts.
Viewport: desktop.

## Procedure and assertions

1. Context A creates a local connection. Context B authenticates and selects the
   same instance. Both receive the same snapshot and subsequent output.
2. Focus/click A's terminal, which claims control, then send marker A. Both views
   display it once. Without claiming control in B, a synthetic stale-client
   input frame is ignored and does not close either socket.
3. Focus/click B and send marker B. Control transfers atomically; both views see
   it. A stale non-owner input is ignored until A explicitly reclaims control.
4. Resize B after it owns control. The authoritative PTY geometry follows B;
   non-owner resize from A does not override it. Reclaim in A and verify the
   reverse.
5. Open read-only hover previews in one context. Previews receive output but
   never claim control, resize the PTY, or consume client capacity indefinitely
   after hover ends.
6. Close one browser context. The connection process and other browser remain
   usable. Sign out/revoke one auth session without terminating the shared
   backend connection.

## Pass gate

Correlate WebSocket frames by context and instance. Fail on cross-instance
input, both clients acting as owner simultaneously, unexpected policy closes,
or any global diagnostics violation in either browser.
