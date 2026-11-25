# Backup and Recovery

## What to back up

Back up the database **with** the durable local state. The default unified PVC
layout is below; standalone deployments should back up the equivalent mounts.

```text
state/      -> /home/roaminal/.roaminal
workspace/  -> /workspace
ssh/        -> /home/roaminal/.ssh
```

- Back up the complete PVC, including `state/database-binding.json`. Also back
  up any separately mounted SSH config, Includes, keys, and known hosts.
- For SQLite, include `state/db/roaminal.sqlite3` and its containing directory
  (or the configured SQLite directory for a standalone deployment). Use a
  consistent offline snapshot or a SQLite-aware backup; do not copy only the
  live `.sqlite3` file.
- For PostgreSQL, back up the database separately from the Roaminal PVC. Retain
  the DSN Secret and CA configuration. For CloudNativePG, back up the managed
  credential Secret and stable bootstrap marker with the database. Do not
  regenerate credentials against initialized storage; see
  [managed recovery](../chart/README.md#managed-postgresql).
- Keep the stable login password and, if enabled, Web Push VAPID credentials
  in their existing secret stores and include them in your recovery plan.

Treat databases, exports, PVCs, and native SSH backups as credential material.
The database includes the TOTP secret, login session records, and terminal
snapshots. Encrypt backups at rest, restrict access, and keep them out of support
bundles. Container layers, preview caches, and upload staging need no backup.

## Restore

[Stop Roaminal](deployment.md#offline-maintenance) before taking a coordinated
copy or restoring. Restore the matching database and local state, including the
binding, from the same recovery point. Preserve ownership and private permissions
(UID/GID 1000 in the image, `0700` for private directories and `0600` for private
files). Do not delete or edit the binding to bypass an installation mismatch.
Use a compatible runtime image and verify login and `/readyz` before accepting
traffic.

Existing connection instances are retired on startup, not reopened. Interrupted
uploads must be started again from their source files. A password change revokes
existing login sessions. To reset enrollment without restoring older data, use
[offline TOTP reset](security.md#resetting-totp-enrollment).

## Switching databases

To move between SQLite and PostgreSQL, or restore a logical export into a new
DB, keep Roaminal stopped throughout. Back up the source DB and matching PVC
or local state first. Use the same [maintenance mounts](deployment.md#offline-maintenance)
for all three commands.

1. With the **source** DB settings, export from the bound database. The output
   must be a new file on private storage. Export refuses pending connection
   definition mutations.

   ```sh
   roaminal db export --output /mnt/backup/roaminal.export
   ```

2. Switch the maintenance environment and CA mount to the **destination**, but
   keep the **same durable state mount and binding**. Import into a new, empty
   database that has never served. Import checks the export and reads back the
   destination records. It requires a private regular file (keep mode `0600`).
   An identical retry succeeds only if the destination still matches; it does
   not repair or overwrite changes.

   ```sh
   roaminal db import --input /mnt/backup/roaminal.export
   roaminal db activate
   ```

   Activation requires the export's exact source database ID to match
   `database-binding.json` on that local state. It verifies the current
   destination records before updating the binding; first startup verifies
   them again before changing any application records. If the destination no
   longer matches, keep Roaminal stopped and restore into a clean database.
   An export alone cannot replace the binding, workspace, or SSH files.
3. Only after activation, restart with the destination DB settings. The
   activated database is authoritative; legacy files on the state mount are
   no longer re-imported or checked as a source of live records.

If a command fails, keep Roaminal stopped. To roll back after activation,
restore the source DB and matching pre-transfer local state (including its
binding). To preserve new writes, transfer back into a new target instead.
Switching Helm values alone does not roll back data.
