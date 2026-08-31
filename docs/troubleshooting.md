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
password or restarting with a generated password revokes existing login sessions.

## WebSocket closes immediately

Use the current page Origin, `ws:`/`wss:` matching the page, and both
`roaminal.v2` and `roaminal.auth.<access-token>` subprotocols. Proxies must pass
the Upgrade request, preserve the external Host, and keep connections open for
at least an hour. A proxy may report the WebSocket transport as `ws`/`wss` in
`X-Forwarded-Proto`; Roaminal maps those values to the browser's `http`/`https`
Origin scheme. `1008`
indicates an invalid JSON message or schema; `1009` is an oversized message and
`1013` is a slow-client queue overflow.

If the response is `403` with `{"code":"origin_denied"}`, compare the Host and
Origin values on the Upgrade request. The HTTP and WebSocket routes must use the
same external host and TLS termination settings. Do not disable Origin checks;
fix the proxy's WebSocket route instead.

## FileSystem is unavailable

FileSystem requires a live SSH connection instance with a source host alias.
The root is first resolved from the active tmux pane, with one retry per
request, and then falls back to the connection definition's
`filesystem.pwd` (default `$HOME`). A
`filesystem_no_transport` or
`filesystem_transport_unavailable`
response means the instance has no usable remote transport; reconnect the
instance and resolve the FileSystem root again. A root revision conflict means
the pane directory changed; resolve the root before listing or reading entries.

## Agent initialization or messages fail

Agent initialization requires a live SSH tmux connection and remote `tmux`
and Codex. Check the Agent status response, then verify that the remote
platform is Linux or macOS on `amd64` or `arm64`. The initialization operation
ID and phase in the application log identify whether transport acquisition,
platform detection, existing-component probing, upload, or installation
failed. After a failed installation, inspect the remote component prerequisites
and retry; repeated initialization for the same endpoint joins the active
operation.

Once initialized, the installed Agent component must be executable, its
component metadata must remain mode `0600`, and its local state/log directories
must remain private.
The message center reports actual standard Agent state transitions, not every
hook event. A missing transition should be correlated with
`$HOME/.roaminal/logs/codex-hook.log`, the configured tmux session, and the
server's `agent_state_sync_failed` log. The local hook log records the tmux
session identity, state index allocation, and local I/O failures without
recording credentials or terminal content. It keeps the current and one
rotated segment within a combined 128 MiB budget and removes segments older
than 48 hours. The hook uses an OS-managed per-tmux file lock; stale
`tmux wait-for` locks from older component versions are no longer used.

## Browser diagnostics

When client diagnostics are enabled, search the application log for
`client_diagnostic=` and inspect the redacted NDJSON files below
`<state-root>/diagnostics/`. Correlate `pageId`, `eventId`, `bootId`, and the
runtime version with the browser and proxy timestamps. Review application-
authored messages for sensitive material before sharing a record; diagnostics
never include terminal content or SSH credentials by design, but arbitrary
application error strings are retained after pattern redaction.

The browser cannot expose every DevTools network message. For a WebSocket
handshake failure, use the reported endpoint/phase and the corresponding
server or proxy access log; the browser event does not invent an HTTP status.

Production stacks may reference minified assets such as `index-<hash>.js:19`.
Download the matching private GitHub Actions artifact
`roaminal-frontend-sourcemaps-<runtime-version>`, verify its manifest SHA-256
values, and use the included JavaScript and `.map` files with a source-map
viewer. The runtime image intentionally does not serve `.map` files.

## PVC pod is not ready

Confirm the unified RWO PVC is bound and its root is writable by UID/GID 1000.
The init container creates `state/`, `workspace/`, and `ssh/`; it does not repair
ownership or permissions on an existing mount. A read-only SSH Secret/projected
volume is valid, but `/workspace` and the state subpath must remain writable.
Inspect probe events and `kubectl logs`; do not expose or edit the worker process
directly.
