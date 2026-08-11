# Backup and Recovery

The complete persistent state is the unified PVC used by the Helm Chart. Its
logical directories are:

```text
state/      -> /home/roaminal/.roaminal
workspace/  -> /workspace
ssh/        -> /home/roaminal/.ssh
```

The state directory contains:

```text
auth-sessions.json
connection-instances/<uuid>/metadata.json
connection-instances/<uuid>/terminal.snapshot
audit/connection-instances/<uuid>/metadata.json
audit/connection-instances/<uuid>/terminal.snapshot
```

活动 connection 退出后，Roaminal 先将最新 metadata 和 terminal snapshot 复制到
`audit/connection-instances/<uuid>/`，再删除 `connection-instances/<uuid>/`。
审计材料目前只作为保留副本，不提供 UI 或查询 API；没有 snapshot 时只保留 metadata。
因此活动目录不会积累可重新连接的历史记录。

The `ssh/` directory contains user-managed SSH config, keys, and known-hosts
data. The complete unified PVC must be backed up and protected as
high-sensitivity credential material; never include it in support bundles or
ordinary diagnostics. If SSH is supplied by a read-only Secret or projected
volume, that external source must be backed up separately as well.

If a storage driver exposes the PVC mount root as an fsGroup-owned directory
that the non-root process cannot chmod, Roaminal places the same layout below
`.roaminal/state/`. Back up the entire mounted directory in either case; the
application-owned state directory remains `0700` and its files remain `0600`.

Back up the unified PVC while the service is stopped, or use a filesystem
snapshot that guarantees a consistent directory image. Treat the backup as
secret material because it contains SSH keys, refresh-token hashes, user-agent
metadata, session metadata, workspace files, and terminal scrollback. Do not
back up container layers or the ephemeral worker process.

To restore, stop the deployment, restore the unified directory tree with owner
UID/GID 1000 and modes `0700`/`0600`, then start the same Git-SHA or a compatible
newer image. The top-level `state/`, `workspace/`, and `ssh/` directories must
remain intact.
Roaminal verifies snapshot magic, schema, length, UTF-8, and SHA-256. A corrupt
snapshot is renamed with a UTC `.corrupt.<timestamp>` suffix; its session
metadata remains and a fresh Bash starts with empty scrollback. A corrupt auth
file is quarantined and requires a new login.

Changing `ROAMINAL_PASSWORD` intentionally invalidates all prior refresh
sessions. A generated password changes on every restart, so use an explicit
Secret when authentication continuity is required.
