# PW-CONN-011: SSH source changes and transport draining

Priority: P1. Capabilities: mutable SSH config and SSH transport. Viewport:
desktop. Run serially.

Fixture: complete the [SSH and tmux codespace](../fixtures/ssh-codespace.md)
with a writable Roaminal SSH config before this case.

## Procedure and assertions

1. Start A and an unrelated B from separate definitions. Prove both SSH
   channels, FileSystem root access, and remote-monitor polling are live before
   changing any source.
2. Change only B's supported config. Existing A remains current and its
   terminal, FileSystem, and remote-monitor requests continue to work while B
   becomes changed after the transport watcher reconciles configuration.
3. Start B again. Roaminal must not silently reuse B's now-stale transport.
   The request either creates a new transport from current config or returns the
   documented draining conflict; the UI shows one actionable error and remains
   usable.
4. After B's relevant edit, verify its existing instance still has auxiliary
   FileSystem and remote-monitor access, while it cannot be selected as a new
   reuse anchor. A fresh B start uses a new transport.
5. Delete B while its historical instance is live. It remains an explicit live
   instance with deleted source state until it exits, and its existing
   FileSystem/monitor access remains available while the control socket is
   healthy. The manager no longer offers the deleted definition.
6. Restore/recreate B and Start. The fresh transport does not attach to the
   deleted historical one. A's mapping, monitor, and FileSystem access remain
   intact throughout all B operations.

## Cleanup and pass gate

Close all test instances and restore the config fixture. Only the explicitly
documented draining response is allowed; unexpected non-retryable transport
errors, new credential prompts from a no-fallback reuse attempt, or any
`no_remote_transport`/`filesystem_no_transport` response for an existing live
instance fail the case.
