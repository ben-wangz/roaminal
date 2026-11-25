# Security

Run Roaminal behind an authenticated, same-origin HTTPS proxy. The Go backend is
the only network listener; the terminal worker uses private stdin/stdout.
Proxies must preserve WebSocket upgrades and allow one-hour read/send timeouts.

Login uses a 30-second, single-use HMAC-SHA256 challenge plus a mandatory
TOTP second factor. A valid password proof yields only a five-minute,
purpose-bound pending token: before enrollment it may open TOTP setup only,
after enrollment only TOTP verification. Normal access and refresh tokens are
issued after a verified code. The browser retains access and refresh tokens
only in origin-local storage; pending tokens, setup secrets, and codes stay
in component memory. The database stores refresh-token hashes, not plaintext
refresh tokens. Refresh rotates tokens and requires the login session to stay
bound to the current password and TOTP enrollment. Changing the password
revokes prior login sessions.

## Resetting TOTP enrollment

If the authenticator is lost, [stop Roaminal and use offline maintenance](deployment.md#offline-maintenance)
with the same durable state mount and the existing, bound database. Run:

```sh
roaminal admin totp reset --confirm
```

The reset clears the database's enrollment, replay state, and all existing
login sessions in one transaction. It does not delete connection definitions
or workspace data. Restart with the same DB configuration and sign in with the
password to enroll a new authenticator. Deleting the legacy `2fa-secret` file
does not reset enrollment. Database errors deny login; they do not trigger
re-enrollment.

Protect the authentication Secret, database credentials, local mounts, and
backups. The database and logical exports contain the TOTP secret and terminal
snapshots; the PVC can contain workspace files and SSH keys. Never put tokens
in URLs, logs, screenshots, reports, or proxy access logs.

The container runs as UID/GID 1000 with a read-only root filesystem, no added
capabilities, no privilege escalation, and no host/runtime socket. Never expose
the worker protocol through a Service or production port-forward.

The remote monitor uses only an existing SSH ControlMaster. It disables new
connections, credential prompts, forwarding, and user-supplied remote
commands; collector output is bounded, parsed from an allowlist, and never
persisted. Metrics remain unknown when cgroup ownership cannot be established.

Agent initialization normalizes the SSH endpoint in the backend and identifies
the tmux target by its session name and runtime identity. The installed Agent
component stores only private component metadata and per-runtime Agent state
files under `$HOME/.roaminal/`, with state and lock files at `0600`. It performs
no network access and receives no endpoint, connection, or credential
configuration. The backend reads state files only through the live SSH
connection instance, validates the tmux identity and monotonic index, and
persists only the latest projection.

FileSystem access is limited to backend-controlled probes and transfers below
the resolved root. It has no arbitrary remote-command endpoint.

Client diagnostics are same-origin and require an access token. They collect
redacted browser errors, uncaught rejections, failed resource paths, and
Roaminal WebSocket lifecycle metadata. They never collect terminal input or
output, commands, PWD, SSH configuration, key material, passwords, tokens,
cookies, headers, DOM text, or arbitrary object properties. The server redacts
again, writes one-line records to stdout, and keeps at most five private NDJSON
files (10 MiB total, seven days). Review application-authored error strings
before sharing logs. Production source maps remain private GitHub Actions
artifacts and are not served by the runtime.
