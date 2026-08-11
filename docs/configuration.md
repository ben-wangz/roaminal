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

Terms acknowledgement is required. Explicitly supplied empty passwords are an
error; when no password is supplied a new random password is printed once at
startup, so stable passwords are required for refresh sessions to survive a
restart. Invalid values fail startup rather than being clamped.

The state directory is `~/.roaminal`. It contains the auth file, active
`connection-instances/<id>/metadata.json` and `terminal.snapshot` files, audit
copies under `audit/connection-instances/`, and
`ssh-connection-options.yaml` for Roaminal-only tmux settings. SSH config and
key material remain under `~/.ssh/`.
