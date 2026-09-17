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
in component memory. Authentication sessions persist only refresh-token hashes,
the password fingerprint, and the enrollment binding; the TOTP credential is
kept separately in the private `2fa-secret` file below. Refresh rotates tokens
and is allowed only while the session stays bound to the current enrollment.
Changing the password revokes prior login sessions.

## Resetting TOTP enrollment

The TOTP secret lives in `<state root>/2fa-secret` (`~/.roaminal/2fa-secret`
by default, or its `state/` child in private-child layouts; the same
directory as `auth-sessions.json`). The file holds the Base32 secret, an
enrollment identity, and the last consumed time step; it is created with
`0600` permissions inside a `0700` directory and is never logged, exported,
or sent to generic APIs outside the setup stage.

To reset authentication without losing connection definitions, instances, or
workspace data: delete `2fa-secret` from the running backend filesystem (for
example with `kubectl exec` into the Pod), then log out and log back in. The
next login asks for the password and then offers initial enrollment; no
restart is required. Logout remains usable while old credentials are invalid.
Re-enrollment invalidates every older token, refresh session, and open
stream. A missing file means unconfigured; an empty, malformed, or unreadable
file is an error that denies login until the file is repaired or removed;
the backend also refuses to start against such a file. Deleting or reading
the file is within the operator's authority; the backend does not encrypt it
against process-level filesystem access. Perform deletion while login and
setup are idle: an external delete racing an in-flight enrollment write is
not transactional.

State directories use `0700`; state files use `0600`, fsync, and atomic rename.
Protect the authentication Secret and unified PVC as credential material: the
PVC includes workspace files, SSH keys, and terminal scrollback. Never put
tokens in URLs, logs, screenshots, reports, or proxy access logs.

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
