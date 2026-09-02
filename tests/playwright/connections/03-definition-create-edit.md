# PW-CONN-003: Create and edit an SSH definition

Priority: P0. Capabilities: mutable SSH config. Viewport: desktop. Run serially.

## Procedure and assertions

1. In Settings > Connection definitions, open New connection. Defaults are `User=root`, `Port=22`, and
   `ServerAliveInterval=15`. If an available Ed25519 key exists it is selected
   and `IdentitiesOnly=yes`; otherwise no identity is silently invented.
2. Verify required/limit validation: Connection name follows the supported pattern,
   port is `1..65535`, and numeric inputs reject out-of-range values. A failed
   native or server validation leaves the form open and preserves user input.
3. Create a unique alias with HostName, User, Port, one or more managed identity
   files, IdentitiesOnly, StrictHostKeyChecking, UserKnownHostsFile, and
   ServerAliveInterval. The save request uses the current ETag.
4. Selecting `StrictHostKeyChecking=no` or
   `UserKnownHostsFile=/dev/null` immediately shows the weakened-host-verification
   alert. Defaults do not show the alert.
5. Save and verify one correct Host block exists in `~/.ssh/config`, the manager
   row reflects all supported values and `weakened trust`, and no private-key
   content crosses the browser API.
6. Edit the same row. Fields round-trip exactly, including multiple identities.
   Change values back to safe defaults and save; the row and file update without
   duplicate directives.
7. Expand Advanced options. The tmux session input is disabled until enabled,
   then requires a name beginning with a letter and containing only letters,
   digits, `_`, or `-`, up to 64 characters. Disabling tmux makes it non-editable
   and removes the effective add-on for this alias.

## Cleanup and pass gate

Delete only the definition created by this case and restore any selected key
state. Run the global diagnostics gate and assert the create/edit requests
return updated ETags.
