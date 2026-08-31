# PW-WORK-014: Codex Agent status and initialization

Priority: P1. Capabilities: Kubernetes release, isolated SSH/tmux fixture, and
disposable remote home. Viewport: desktop unless a responsive assertion is
explicitly listed.

## Procedure and assertions

1. Run the mandatory Helm deployment gate and browser diagnostics gate from the
   Playwright README. Use a fresh writable remote home whose fixture has `tmux`
   and `codex` on `PATH`.
2. Create a live SSH connection with tmux enabled. Verify every connection card
   has a Terminal icon button that always opens Terminal, a Files icon when the
   FileSystem capability is available, and a separate pixel-art Codex robot
   status button. The robot uses `data-agent-state` and
   `data-agent-activity`; a supported uninitialized target shows the sleeping
   artwork. Capture the five artwork states and verify their stable asset
   names/accessible labels: sleeping for supported uninitialized, confusing for
   unknown or unsupported/unavailable, singing-relax for idle, busy-working for
   running/initializing, and broken for an explicit setup or Agent error. Local
   and non-tmux connections must show confusing artwork with the
   unknown/unavailable tooltip rather than sleeping.
3. Open the Codex robot status button. Verify the dialog names the connection,
   tmux session, resolved endpoint for display, `$HOME/.roaminal/`, and
   `$HOME/.codex/hooks.json`. Explain that the installed hook writes local Agent
   state only and never sends prompts, transcripts, terminal output, tool data,
   endpoint data, or credentials to Roaminal. Verify no initialization request
   or remote write occurs before confirmation.
4. Confirm initialization. Correlate the authenticated POST to
   `/api/v2/connection-instances/<id>/agent/initializations` with `{}`. Poll the
   operation resource and verify the sidebar progresses through `initializing`
   to `needs_trust`.
5. Before confirming on a disposable fixture, create existing
   `$HOME/.roaminal` and `$HOME/.codex` directories with mode `0755`, and an
   existing `$HOME/.codex/hooks.json` with mode `0644`. Confirm initialization
   repairs them to `0700` and `0600` respectively. Verify the canonical helper
   exists at `$HOME/.roaminal/bin/roaminal-agent-hook`, the local component
   configuration is private, and the user hook config contains exactly one
   canonical command per required Codex event while preserving unrelated
   handlers.
6. Restart Codex in the same tmux session, trust the Roaminal hook through
   `/hooks`, and wait for a hook event. Verify the remote state file is created
   under `$HOME/.roaminal/state/agents/codex/`, the state identity contains the
   exact tmux session identity, and no request body or diagnostic output exposes
   prompts, transcripts, cwd, model, tool arguments, tool output, endpoint
   credentials, or network URLs.
7. Verify the compatibility mapping through the UI and heartbeat projection:
   SessionStart/resume/clear settles on `relax`; prompt, permission, tool, and
   compact activity reports `running`; Stop and an unclassified SessionEnd
   report `relax`. A tool-level failure and Ctrl-C interruption must not be
   guessed as Agent `error` on the current Codex provider. The UI must keep
   synchronization failures separate from Agent `error`.
8. Generate more than 128 local hook events and verify the state file retains
   the newest 128 records in increasing index order. Restart Roaminal and verify
   the backend restores only the latest snapshot, not the local history. Repeat
   with a reused tmux session name after creating a new tmux runtime and verify
   the new runtime starts a separate index stream and an old delayed snapshot
   cannot replace it.
9. Refresh the page and verify component readiness and the latest Agent state
   survive. Open Initialize or Repair twice concurrently from two browser tabs
   and assert one endpoint operation is shared, the component converges, and
   every canonical hook command remains present only once. Repeating the action
   after success must be safe.
10. Create a second tmux connection on the same SSH endpoint with another
    session name. Verify component readiness is shared but Agent state is
    isolated. Reuse the first tmux session from another connection instance and
    verify both live instances receive the same projection and one transition
    message. Repeat with a second endpoint using the same session name and
    verify no state crosses endpoints.
11. Exercise the synchronization failures: missing state, missing configured
    tmux session, unavailable transport, unreadable state, malformed state,
    unsupported capability, and an old snapshot. Verify the next 60-second
    cycle retries independently, the last valid Agent state is retained, the
    UI uses a separate confusing/stale/unavailable indication, and no failure
    transition or browser notification is invented.
12. Verify the local diagnostic log is private, redacted, bounded to the
    documented 128 MiB total limit, and cleaned according to the documented
    retention policy. Hook I/O or tmux discovery failures must not block Codex
    because the hook is best effort.

## Pass gate and cleanup

Correlate every protected API response with the action that caused it. Fail on
unexpected browser diagnostics, leaked provider payload fields, duplicate
initialization operations, stale cross-runtime state, automatic command
execution, network access from the hook, or automatic fullscreen entry. The
independent fullscreen control is covered by its dedicated regression case.
Capture screenshots of `uninitialized`, `needs_trust`, `ready`, `running`, and
the separate synchronization-unavailable state.

Delete only connection instances, tmux sessions, and remote files created by
the case, then reset the disposable fixture home.
