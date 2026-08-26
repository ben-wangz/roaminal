# PW-WS-015: Agent message center

Priority: P1. Capabilities: Kubernetes release, isolated SSH/tmux fixture, a
fixture with the Roaminal Agent component installed and trusted, and a
disposable remote home. Viewports: desktop, tablet, phone, and 320 px.

## Procedure and assertions

1. Run the mandatory Helm deployment and browser diagnostics gates from the
   Playwright README. Create a live SSH connection with tmux enabled, install
   the Codex Agent component, trust the hook, and verify the helper can post a
   valid metadata-only event.
2. On the workspace, Connections page, and Appearance page verify a Lucide
   Bell control appears immediately before Appearance. Its label is `Messages`
   when empty and includes the unread count otherwise. The badge has no zero
   state and displays `99+` for counts above 99.
3. Trigger the first accepted hook event and verify exactly one history row
   says `Codex reporting connected`. Refresh the page and open a second
   authenticated tab; the row remains in history and no transient notice is
   replayed by either page.
4. Submit one Codex turn and verify one new row says `Codex turn finished`,
   with the correct connection label and a success icon. Replay the hook event,
   retry it after a lost response, and verify the history and notice counts do
   not increase. Verify SessionStart, SessionEnd, tool errors, stale state,
   disconnects, and rejected or late events never create a failed row.
5. Create a second live connection instance that reuses the same tmux target.
   Trigger another accepted event and verify one row is created with both
   instance IDs, the selected live label includes `+1`, and clicking the row
   selects the displayed instance and opens Terminal. Retire one instance and
   verify a remaining live instance is selected; retire both and verify the
   fallback endpoint/tmux label remains readable and is not navigable.
6. Open the Bell and verify the nonmodal panel is below the topbar, has one
   scrolling list, shows newest first, loads 50 rows initially, and loads an
   older page at the scroll end. Verify the empty state is exactly `No messages
   yet.` on a fresh fixture. Opening the panel marks loaded messages read;
   `Mark all read` advances the global cursor. In two tabs, verify read state
   never moves backwards and synchronizes after the next heartbeat revision.
7. Generate more than three messages together. Verify transient notices show
   at most the newest two plus one summary such as `3 more Agent messages`.
   Info/success notices expire after six seconds and an explicit failed fixture
   (only if supported) expires after ten seconds. Dismissal leaves the durable
   row unread; clicking a notice marks it read and navigates when a live target
   exists. The existing bottom-right operation toast remains visible and
   independent.
8. At phone and tablet widths verify the Bell remains in the topbar and the
   message panel is a fixed panel with 8 px side margins below the topbar, not
   a bottom sheet. Open the native software keyboard through the terminal
   helper, verify the panel closes and notices are suppressed without moving
   focus from the helper textarea, then create a message and verify at most the
   newest queued notice appears after the keyboard closes.
9. Inspect request and browser diagnostics. Message responses must not contain
   endpoint keys, Agent event IDs, Codex session or turn IDs, tmux socket
   fingerprints, tokens, webhook URLs, prompts, transcripts, cwd, models,
   command output, tool arguments, or tool output. Invalid cursors return
   `message_cursor_invalid`; malformed read state returns
   `message_read_state_invalid`; a storage outage returns retryable
   `message_store_unavailable` without acknowledging the Agent event.
10. Verify Escape and outside pointer close the panel and return focus to the
    Bell, icon-only controls have accessible names and tooltips, status is not
    communicated by color alone, reduced motion is understandable, and the
    320 px viewport has no horizontal document overflow.

## Pass gate and cleanup

Correlate every message API response with the event or UI action that caused
it. Fail on duplicate durable messages, replayed hydration notices, incorrect
shared-target attribution, read-state regression, leaked metadata, unexpected
browser diagnostics, or any command/fullscreen UI. Capture screenshots of the
Bell with an unread badge, the open history panel, a transient notice, the
shared-target `+1` label, and the keyboard-suppressed phone layout. Delete only
connection instances, tmux sessions, and remote files created by the case,
then reset the disposable fixture home.
