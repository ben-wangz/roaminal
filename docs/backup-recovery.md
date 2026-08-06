# Backup and Recovery

The complete persistent state is the `.roaminal` directory:

```text
auth-sessions.json
sessions/<uuid>.json
sessions/<uuid>.snapshot
```

Back up the state volume while the service is stopped, or use a filesystem
snapshot that guarantees a consistent directory image. Treat the backup as
secret material because it contains refresh-token hashes, user-agent metadata,
session metadata, and terminal scrollback. Do not back up container layers or
the ephemeral worker process.

To restore, stop the deployment, restore the directory with owner UID/GID 1000
and modes `0700`/`0600`, then start the same Git-SHA or a compatible newer image.
Roaminal verifies snapshot magic, schema, length, UTF-8, and SHA-256. A corrupt
snapshot is renamed with a UTC `.corrupt.<timestamp>` suffix; its session
metadata remains and a fresh Bash starts with empty scrollback. A corrupt auth
file is quarantined and requires a new login.

Changing `ROAMINAL_PASSWORD` intentionally invalidates all prior refresh
sessions. A generated password changes on every restart, so use an explicit
Secret when authentication continuity is required.
