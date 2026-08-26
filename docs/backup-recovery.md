# Backup and Recovery

Back up the complete unified PVC. It contains:

```text
state/      -> /home/roaminal/.roaminal
workspace/  -> /workspace
ssh/        -> /home/roaminal/.ssh
```

`state/` stores authentication sessions, per-login-session workspace
layouts, Agent endpoint bindings and token hashes, message history, upload
records, active connection metadata and snapshots, and archived
metadata/snapshots under `audit/connection-instances/`. It also contains
bounded browser diagnostics and temporary upload staging. When a connection
exits, its active files are archived and removed; archive data is not
reconnectable and has no UI or API.

Treat the PVC and any SSH Secret/projected source as credential material. Do
not include them in support bundles. If the storage mount cannot be chmodded by
UID/GID 1000, state is stored below `.roaminal/state/`; back up the whole mount.

## Backup

Stop Roaminal before copying the PVC, or use a storage snapshot with filesystem
consistency. Preserve UID/GID 1000, directories at `0700`, and files at `0600`.
Container layers and the terminal worker are ephemeral and need no backup. An
in-progress FileSystem upload is not resumed after a backend restart; retain
the source files and start it again after recovery.

## Restore

Stop the release, restore the full directory tree, then start the same or a
newer compatible image. Preserve the top-level `state/`, `workspace/`, and
`ssh/` directories. On startup, active connection instances are archived and
retired; they are not reopened. Audit copies remain available only as backup
material. Changing `ROAMINAL_PASSWORD`, or restarting with a generated
password, invalidates existing refresh sessions.

Corrupt state is quarantined and reported through the degraded-persistence
status. A corrupt auth file requires login; a corrupt terminal snapshot is not
used to recreate a connection instance.
