# PW-AUTH-002: Access-token refresh during an active workspace

Priority: P0. Capabilities: core plus a test release with a short access TTL.
Viewport: desktop. Run serially.

## Preconditions

- Configure the dedicated release with an access TTL short enough to expire
  during the case and a refresh TTL long enough for the full run.
- Authenticate, create a local connection, and wait for its WebSocket and
  heartbeat to be stable.

## Procedure and assertions

1. Write a unique marker through the terminal and confirm its output.
2. Keep the page open past the original access-token expiry while heartbeat
   polling continues. Observe one coordinated refresh request rather than a
   burst of refreshes.
3. Verify the refresh response rotates both tokens in local storage. The old
   token must not be reused by later HTTP requests or a newly created preview or
   main terminal WebSocket.
4. After refresh, switch between two connections, open and close a sidebar
   preview, issue another terminal command, open Connections, and return to the
   workspace. All operations must work without re-login.
5. Leave the page active for at least one further heartbeat/monitor interval and
   verify there is no repeating unauthorized-refresh loop.

## Pass gate

No browser diagnostic may contain `HTTP Authentication failed; no valid
credentials available`, an unhandled `401`, or a WebSocket opened with the
expired token. Record refresh count and the affected request paths without
recording token values.
