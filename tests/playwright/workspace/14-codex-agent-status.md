# PW-WS-014: Codex Agent status and initialization

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
   `data-agent-activity`; `uninitialized` shows the sleeping artwork. Local and
   non-tmux connections show the unknown/unavailable robot tooltip.
3. Open the Codex robot status button. Verify the dialog names the connection, tmux session,
   resolved endpoint, `$HOME/.roaminal/`, `$HOME/.codex/hooks.json`, webhook
   URL, and metadata-only privacy boundary. Verify no initialization request or
   remote write occurs before confirmation.
4. Confirm initialization. Correlate the authenticated POST to
   `/api/v2/connection-instances/<id>/agent/initializations` with `{}`. Poll the
   operation resource and verify the sidebar progresses through `initializing`
   to `needs_trust`.
5. Before confirming on a disposable fixture, create existing
   `$HOME/.roaminal` and `$HOME/.codex` directories with mode `0755`, and an
   existing `$HOME/.codex/hooks.json` with mode `0644`. Confirm initialization
   repairs them to `0700` and `0600` respectively, then verify the canonical
   helper exists at
   `$HOME/.roaminal/bin/roaminal-agent-hook`, the config is mode `0600`, and the
   user hook config contains exactly one canonical command per required Codex
   event while preserving unrelated handlers.
6. Restart Codex in the same tmux session, trust the Roaminal hook through
   `/hooks`, and wait for a webhook request. Verify the matching connection
   becomes `ready`; no request body contains a prompt, transcript, cwd, model,
   tool arguments, tool output, token, endpoint, or webhook URL.
7. Trigger events and verify the exact status semantics: UserPromptSubmit is
   `running`, PermissionRequest is `waiting`, PostToolUse returns to `running`,
   Stop is `completed` with `Codex turn finished`, and SessionEnd is `idle`.
   Verify stale thresholds produce `idle`/`stale` and never invent `failed`.
8. Refresh the page and verify component readiness survives. Open Repair twice
   concurrently from two browser tabs and assert one endpoint operation is
   shared, the remote token is not replaced twice, and every canonical hook
   command remains present only once.
9. Create a second tmux connection on the same SSH endpoint with another
   session name. Verify component readiness is shared but activity is isolated.
   Reuse the first tmux session from another connection instance and verify both
   instances display the same activity. Repeat with a second endpoint using the
   same session name and verify no status crosses endpoints.
10. Run the failure cases: invalid token, replayed sequence, oversized event,
    rate limit, temporary webhook outage with spool recovery, and endpoint
    conflict. Verify the UI shows stable safe errors and the remote configuration
    is never silently overwritten.

## Pass gate and cleanup

Correlate every protected API response with the action that caused it. Fail on
unexpected browser diagnostics, leaked event payload fields, duplicate upload
or initialization operations, stale cross-instance state, automatic command
execution, or automatic fullscreen entry. The independent fullscreen control
is covered by its dedicated regression case. Capture screenshots of
`needs_trust`, `ready`, `running`, and `waiting`.
Delete only connection instances, tmux sessions, and remote files created by the
case, then reset the disposable fixture home.
