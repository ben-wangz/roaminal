# PW-CONN-011: SSH source changes and transport draining

Priority: P1. Capabilities: mutable SSH config and SSH transport. Viewport:
desktop. Run serially.

## Procedure and assertions

1. Start A from a definition and prove the SSH channel is live. Edit that
   definition's supported config so its ETag/revision changes.
2. Existing A remains usable, but its source state becomes changed as the
   transport watcher reconciles configuration.
3. Start the row again. Roaminal must not silently reuse A's now-stale transport.
   The request either creates a new transport from current config or returns the
   documented draining conflict; the UI shows one actionable error and remains
   usable.
4. Delete the definition while A is live. A remains an explicit live historical
   instance with deleted source state until it exits; it cannot be selected as a
   reuse anchor and the manager no longer offers the deleted row.
5. Restore/recreate the alias and Start. A fresh transport uses the new source
   revision and does not attach to the deleted one.
6. Change only unrelated Host blocks and verify a connection whose effective
   source revision remains current is not spuriously drained.

## Cleanup and pass gate

Close all test instances and restore the config fixture. Only the explicitly
documented draining response is allowed; unexpected `transport unavailable`,
new credential prompts from a no-fallback reuse attempt, and all global
diagnostic errors fail the case.
