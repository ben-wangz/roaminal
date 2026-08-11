# PW-SEC-002: HTTP and WebSocket authorization boundary

Priority: P0. Capabilities: core. Viewport: desktop. Use disposable browser/API
contexts and never print tokens.

## Procedure and assertions

1. Without auth, verify `/healthz`, `/api/version`, and login challenge endpoints
   have their documented public behavior. Protected heartbeat, instance,
   definition, key, monitor, and auth-session endpoints return `401` JSON errors.
2. From a page with a mismatched/null Origin and through requests with a wrong
   Origin scheme/host, call API and WebSocket paths. They return `403 origin
   denied`. Matching host and effective scheme succeed.
3. Login and verify protected HTTP requests use Bearer auth. Tokens are never
   query parameters. After token rotation, the prior access and refresh tokens
   are rejected while current tokens work.
4. Open an instance WebSocket with both `roaminal.v1` and
   `roaminal.auth.<current-token>`. The server selects only `roaminal.v1` and
   never echoes the auth-bearing subprotocol. Missing, malformed, old, or revoked
   auth is rejected before attach.
5. On a disposable raw browser WebSocket, send malformed JSON, an unknown field,
   an unknown message type, and an oversized message in isolated attempts.
   Verify close codes `1008` for policy/schema violations and `1009` for size;
   the terminal process and a valid browser remain usable.
6. Attempt to attach another auth session to a pending launch owned by the first
   session. It is forbidden. A published normal connection remains shareable
   under the normal multi-browser control rules.
7. Send JSON requests with wrong content type, unknown fields, multiple JSON
   values, and a body over 1 MiB. Validate the documented `400`/`413` behavior
   and that no mutation occurs.

## Pass gate

Every negative response/close must be explicitly matched; exclude it from the
global failure collection only for that substep. Unexpected `5xx`, token leakage,
server-selected auth protocol, or uncaught browser diagnostics fail the case.
