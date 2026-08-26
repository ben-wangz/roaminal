# Security

Run Roaminal behind an authenticated, same-origin HTTPS proxy. The Go backend is
the only network listener; the terminal worker uses private stdin/stdout.
Proxies must preserve WebSocket upgrades and allow one-hour read/send timeouts.

Login uses a 30-second, single-use HMAC-SHA256 challenge. The browser retains
access and refresh tokens only in origin-local storage. The server persists only
refresh-token hashes and a password fingerprint; refresh rotates both tokens.
Changing the password revokes prior refresh sessions.

State directories use `0700`; state files use `0600`, fsync, and atomic rename.
Protect the authentication Secret and unified PVC as credential material: the
PVC includes workspace files, SSH keys, and terminal scrollback. Never put
tokens in URLs, logs, screenshots, reports, or proxy access logs.

The container runs as UID/GID 1000 with a read-only root filesystem, no added
capabilities, no privilege escalation, and no host/runtime socket. Never expose
the worker protocol through a Service or production port-forward.

Remote monitoring uses only an existing SSH ControlMaster. It disables new
connections, credential prompts, forwarding, and user-supplied remote
commands; collector output is bounded, parsed from an allowlist, and never
persisted. Metrics remain unknown when cgroup ownership cannot be established.

Agent initialization binds an endpoint normalized from SSH user, host, and
port, then identifies the tmux target by its session name and tmux identity.
The remote hook stores its bearer token in
`$HOME/.roaminal/agent.json` with mode `0600`; the backend stores token
hashes, not raw tokens. Agent events use a strict bounded schema with
sequence/deduplication checks. Webhook URLs use HTTPS by default; insecure HTTP
requires an explicit configuration opt-in and is limited to loopback unless
that policy is deliberately relaxed.

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
