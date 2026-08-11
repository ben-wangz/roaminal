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
connections, credential prompts, forwarding, and remote commands; collector
output is bounded, parsed from an allowlist, and never persisted. Metrics remain
unknown when cgroup ownership cannot be established.
