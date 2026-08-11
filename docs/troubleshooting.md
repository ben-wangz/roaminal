# Troubleshooting

## `/healthz` is `503`

The terminal worker has not completed its handshake or has exited. Check the
application log and pod events, then inspect the worker path and image contents.
The process deliberately fails fast so the container restart policy can recover
it; it does not hot-restart a broken worker in place.

## Startup rejects configuration

Check `ROAMINAL_ACCEPT_TERMS=true`, a non-empty password, an existing absolute
`ROAMINAL_CWD`, and the numeric/duration bounds in
[configuration](configuration.md). A port already in use is reported as a
startup error and is never moved automatically.

## Login or refresh fails

Use the challenge returned immediately before login. Challenges expire after 30
seconds and are single-use, including failed attempts. Refresh tokens rotate;
discard the old token after a successful refresh. Changing the configured
password or restarting with a generated password revokes old refresh sessions.

## WebSocket closes immediately

Use the current page Origin, `ws:`/`wss:` matching the page, and both
`roaminal.v1` and `roaminal.auth.<access-token>` subprotocols. Proxies must pass
the Upgrade request and keep connections open for at least an hour. `1008`
indicates an invalid JSON message or schema; `1009` is an oversized message and
`1013` is a slow-client queue overflow.

## PVC pod is not ready

Confirm the unified RWO PVC is bound and its root is writable by UID/GID 1000.
The init container creates `state/`, `workspace/`, and `ssh/`; it does not repair
ownership or permissions on an existing mount. A read-only SSH Secret/projected
volume is valid, but `/workspace` and the state subpath must remain writable.
Inspect probe events and `kubectl logs`; do not expose or edit the worker process
directly.
