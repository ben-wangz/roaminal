# PW-AUTH-003: Login-session review and revocation

Priority: P0. Capabilities: core and two independent browser contexts. Viewport:
desktop. Run serially.

## Procedure and assertions

1. Log in from contexts A and B with distinct user agents. In A, open Sessions.
2. The `Login sessions` dialog lists both entries, identifies `This browser` and
   `Other browser`, shows a last-seen time and a shortened session ID, and never
   exposes refresh-token hashes or token values.
3. Revoke B from A. The row disables while busy, disappears after a successful
   DELETE, and B is rejected on its next authenticated request/heartbeat and
   returns to the login surface after refresh fails.
4. Log B in again. From A choose `Log out other sessions`. Only A remains and B
   loses access as above.
5. Open a third disposable context C, then revoke A's current row from A. A must
   sign out locally and server-side; C remains valid.
6. Validate keyboard dismissal and the Close button do not mutate any sessions.
7. On a separate release with a short refresh TTL, create A and B, wait past
   expiry, and reopen/list sessions from a newly authenticated context. Expired
   entries are absent from the UI and persisted session store; an expired
   browser cannot refresh or attach a new WebSocket. Access-token expiry alone
   must not remove a still-valid refresh session.

## Cleanup and pass gate

Sign out all surviving contexts. The global diagnostics gate still applies;
the explicitly induced authorization failures in revoked contexts must be
correlated to revocation and handled by the UI, not surfaced as uncaught console
errors.
