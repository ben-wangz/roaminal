# Configuration

Configuration is applied in this order: built-in defaults, `~/.roaminal/config.json`,
`./config.json`, CLI arguments, then environment variables. Only the canonical
Roaminal fields are accepted. Durations use Go syntax.

| Field | CLI | Environment | Default |
| --- | --- | --- | --- |
| `host` | `--host`/`-h` | `ROAMINAL_HOST` | `127.0.0.1` |
| `port` | `--port`/`-p` | `ROAMINAL_PORT` | `9846` |
| `password` | `--password`/`-a` | `ROAMINAL_PASSWORD` | random 32 chars |
| `websocketPingInterval` | `--websocket-ping` | `ROAMINAL_WEBSOCKET_PING_INTERVAL` | `10s` |
| `scrollbackLines` | `--scrollback-lines` | `ROAMINAL_SCROLLBACK_LINES` | `1000` |
| `maxConnectionInstances` | `--max-connection-instances` | `ROAMINAL_MAX_CONNECTION_INSTANCES` | `32` |
| `maxClientsPerConnectionInstance` | `--max-clients-per-connection-instance` | `ROAMINAL_MAX_CLIENTS_PER_CONNECTION_INSTANCE` | `8` |
| `debug` | `--debug`/`-d` | `ROAMINAL_DEBUG` | `false` |
| `acceptTerms` | `--accept-terms`/`-y` | `ROAMINAL_ACCEPT_TERMS` | `false` |
| `initialCwd` | `--cwd` | `ROAMINAL_CWD` | `/workspace` |
| `frontendDir` | `--frontend-dir` | `ROAMINAL_FRONTEND_DIR` | `../frontend/dist` |
| `authAccessTTL` | `--auth-access-ttl` | `ROAMINAL_AUTH_ACCESS_TTL` | `15m` |
| `authRefreshTTL` | `--auth-refresh-ttl` | `ROAMINAL_AUTH_REFRESH_TTL` | `2160h` |
| `authMaxAttempts` | `--auth-max-attempts` | `ROAMINAL_AUTH_MAX_ATTEMPTS` | `30` |
| `clientDiagnosticsEnabled` | `--client-diagnostics=<bool>` | `ROAMINAL_CLIENT_DIAGNOSTICS_ENABLED` | `true` |
| `browserEnabled` | `--browser-enabled=<bool>` | `ROAMINAL_BROWSER_ENABLED` | `false` |
| `browserWorkerPath` | `--browser-worker-path` | `ROAMINAL_BROWSER_WORKER_PATH` | `/opt/roaminal/browser-worker/index.mjs` |
| `browserChromiumPath` | `--browser-chromium-path` | `ROAMINAL_BROWSER_CHROMIUM_PATH` | `/usr/bin/chromium` |
| `agentHooksDir` | `--agent-hooks-dir` | `ROAMINAL_AGENT_HOOKS_DIR` | `/opt/roaminal/agents/hooks` |
| `webPushVapidPublicKey` | `--web-push-vapid-public-key` | `ROAMINAL_WEB_PUSH_VAPID_PUBLIC_KEY` | empty; disables Web Push when all three fields are empty |
| `webPushVapidPrivateKey` | `--web-push-vapid-private-key` | `ROAMINAL_WEB_PUSH_VAPID_PRIVATE_KEY` | empty |
| `webPushSubject` | `--web-push-subject` | `ROAMINAL_WEB_PUSH_SUBJECT` | empty |
| `filesystemImagePreviewCacheDir` | `--filesystem-image-preview-cache-dir` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_DIR` | `/var/cache/roaminal/filesystem-image-previews` |
| `filesystemImagePreviewCacheTargetMiB` | `--filesystem-image-preview-cache-target-mib` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_TARGET_MIB` | `128` |
| `filesystemImagePreviewCacheMaxAge` | `--filesystem-image-preview-cache-max-age` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_MAX_AGE` | `24h` |
| `filesystemImagePreviewCacheCleanupInterval` | `--filesystem-image-preview-cache-cleanup-interval` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_CLEANUP_INTERVAL` | `10m` |
| `filesystemImagePreviewMaxConversions` | `--filesystem-image-preview-max-conversions` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_CONVERSIONS` | `1` |
| `filesystemImagePreviewMaxSourceMiB` | `--filesystem-image-preview-max-source-mib` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_SOURCE_MIB` | `32` |
| `filesystemImagePreviewMaxOutputMiB` | `--filesystem-image-preview-max-output-mib` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_OUTPUT_MIB` | `16` |
| `filesystemImagePreviewMaxStaticPixels` | `--filesystem-image-preview-max-static-pixels` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_STATIC_PIXELS` | `100000000` |
| `filesystemImagePreviewMaxFrames` | `--filesystem-image-preview-max-frames` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_FRAMES` | `200` |
| `filesystemImagePreviewMaxAnimatedPixels` | `--filesystem-image-preview-max-animated-pixels` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_ANIMATED_PIXELS` | `200000000` |
| `filesystemImagePreviewConversionTimeout` | `--filesystem-image-preview-conversion-timeout` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CONVERSION_TIMEOUT` | `30s` |

Terms acknowledgement is required. Explicitly supplied empty passwords are an
error; when no password is supplied a new random password is printed once at
startup, so stable passwords are required for login sessions to survive a
restart. Invalid values fail startup rather than being clamped.

## Database and local state

Database settings are environment-only:

| Environment | Behavior |
| --- | --- |
| `ROAMINAL_DB_DRIVER` | `sqlite` when unset or empty; set `postgres` to opt in. |
| `ROAMINAL_DB_SQLITE_PATH` | SQLite only. An absolute path; defaults to `$HOME/.roaminal/db/roaminal.sqlite3` (`/home/roaminal/.roaminal/db/roaminal.sqlite3` in the runtime image). |
| `ROAMINAL_DB_POSTGRES_DSN` | PostgreSQL only. A full `postgres://` or `postgresql://` TCP URI with `sslmode=verify-full`. |

Leave the inactive driver's setting unset or empty. Invalid settings fail
startup; Roaminal does not fall back to another driver. Put the SQLite database
and its sidecar files on a durable directory mount with SQLite-compatible
locking and a private database directory (`0700`).

In Helm, use `database.*` values rather than `extraEnv` or `extraEnvFrom` for
DB settings. Keep the PostgreSQL DSN in a same-namespace Secret, not in values,
ConfigMaps, or logs. If using a CA Secret, the Chart mounts it at
`/etc/roaminal/postgres-ca/ca.crt`; use that path for `sslrootcert` in the DSN.
A Secret change does not update running environment variables. Coordinate DB
credential rotation, the Secret update, and a controlled restart. See the
[Chart database options](../chart/README.md#database).

The database stores login sessions and TOTP enrollment, workspace layouts,
connection instance records and snapshots (including audit copies), Agent
projections, messages, notification records, upload job records, and Roaminal's
SSH connection options. Keep `$HOME/.roaminal` durable with either driver: it
contains `database-binding.json`, local configuration, bounded browser
diagnostics, and temporary upload staging. The state directory must be writable
by the runtime user, not by other users. Native SSH config and keys stay under
`~/.ssh/`; workspace files remain on their own mount. Chromium profiles are
temporary under `/tmp`. See [backup and recovery](backup-recovery.md).

## Feature configuration

The Agent component bundle directory is read-only input to Agent initialization
and normally comes from the container image. The installed component uses only
the remote user's local `$HOME/.roaminal/` state, lock, and diagnostic paths;
no server URL or connection-specific configuration is required.

Web Push is disabled unless `webPushVapidPublicKey`,
`webPushVapidPrivateKey`, and `webPushSubject` are configured together. For
Helm, put these values in an existing Secret and reference it with
`webPush.existingSecret`; the chart maps its configured keys to the three
environment variables and never renders the secret contents.

Image previews use a private disk cache. The cache directory is not a browser
setting and must be outside the state, workspace, SSH, home, and `/tmp`
directories. Cache target, source/output byte limits, pixel/frame limits, and
conversion concurrency are bounded by startup validation. The conversion
deadline must remain below the HTTP write timeout. The WebP format, quality,
effort, and pipeline version are fixed implementation details.
