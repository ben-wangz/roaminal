# Security

Roaminal is intended to run behind an authenticated, same-origin HTTPS route.
The backend binds only its configured address and is the only network listener;
the Node terminal worker communicates over private process stdin/stdout and has
no network endpoint.

Login uses a 30-second, single-use challenge and HMAC-SHA256. The browser holds
only the current access/refresh tokens in origin-local storage; the password and
password proof key never persist. Access tokens are in memory on the server;
only a SHA-256 refresh-token hash and a password fingerprint are persisted.
Refresh rotates both tokens and invalidates the previous access token.

State directories are mode `0700`; JSON, snapshots, temporary files, and
quarantine copies are mode `0600`, written with fsync and atomic rename. Change
the configured password to revoke prior refresh sessions. Keep the Secret and
state PVC protected with the platform's secret-management and backup controls.

The container runs as UID/GID 1000 with a read-only root filesystem, dropped
Linux capabilities, no privilege escalation, and no host or container-runtime
socket. Mount only the state and workspace volumes. A reverse proxy must enforce
HTTPS, preserve WebSocket upgrades, and use a read/send timeout of at least one
hour for long-lived terminal connections.

Do not put access or refresh tokens in URLs, logs, screenshots, issue reports, or
proxy access logs. The worker protocol is local and must never be exposed by a
Service or port-forward in production.
