# PW-WS-015: Agent message center

Priority: P1. Capabilities: Kubernetes release, isolated SSH/tmux fixture, a
fixture with the Roaminal Agent component installed and trusted, and a
disposable remote home. Viewports: desktop, tablet, phone, and 320 px.

## Procedure and assertions

1. Run the mandatory Helm deployment and browser diagnostics gates from the
   Playwright README. Create a live SSH connection with tmux enabled, install
   the Codex Agent component, trust the hook, and verify the first local state
   snapshot can be read by the backend synchronizer.
2. On the workspace, Connections page, and Appearance page verify a Lucide
   Bell control appears immediately before Appearance. Its label is `Messages`
   when empty and includes the unread count otherwise. The badge has no zero
   state and displays `99+` for counts above 99.
3. Trigger the initial `relax` snapshot and verify no ordinary hook event or
   component-ready event creates a message. Submit one Codex turn so the
   projection changes to `running`, then complete it so it changes to `relax`.
   Verify exactly one history row records `Agent state: running -> relax`, with
   the correct connection label and a success indication.
4. Replay the same state index, retry a failed synchronization, refresh the
   page, and open a second authenticated tab. Verify the durable history and
   transient notice counts do not increase. Tool errors, Ctrl-C, stale state,
   disconnects, and transport failures do not create Agent failure rows. A
   provider that explicitly supports `error` may be used as a separate fixture
   to verify `running -> error`; current Codex must not infer that transition.
5. Create a second live connection instance that reuses the same tmux target.
   Trigger another actual transition and verify one row is created with all
   matching live instance IDs, the selected live label includes `+1`, and
   clicking the row selects a displayed instance and opens Terminal. Retire
   one instance and verify a remaining live instance is selected; retire both
   and verify the fallback endpoint/tmux label remains readable and is not
   navigable.
6. Open the Bell and verify the nonmodal panel is below the topbar, has one
   scrolling list, shows newest first, loads 50 rows initially, and loads an
   older page at the scroll end. Verify the empty state is exactly `No messages
   yet.` on a fresh fixture. Opening the panel marks loaded messages read;
   `Mark all read` advances the global cursor. In two tabs, verify read state
   never moves backwards and synchronizes after the next heartbeat revision.
7. Click a message whose target connection instance is live and verify the
   workspace switches to that connection's Terminal mode. From a message
   associated with multiple live instances, verify the selected live instance
   is preferred and a remaining live instance is selected after the current
   one is retired. Retire every associated instance, click the historical row,
   and verify the exact toast `The connection for this message is no longer
   connected.` appears without changing the active connection or workspace.
8. Delete one history row with its trailing delete control and verify only
   that row disappears, its transient notice disappears, and the row remains
   absent after refresh. Open the clear control, verify the compact inline
   confirmation `Clear all messages?`, cancel it once without data loss, then
   confirm it and verify all rows and notices disappear, the exact empty state
   is `No messages yet.`, and the Bell has no badge. A repeated delete of the
   same ID must not create an error.
9. Generate more than three transitions together. Verify transient notices
   show at most the newest two plus one summary such as `3 more Agent
   messages`. Info/success notices expire after six seconds and an explicit
   error fixture expires after ten seconds. Dismissal leaves the durable row
   unread; clicking a notice marks it read and navigates when a live target
   exists. The existing bottom-right operation toast remains visible and
   independent.
10. At phone and tablet widths verify the Bell remains in the topbar and the
    message panel is a fixed panel with 8 px side margins below the topbar, not
    a bottom sheet. Open the native software keyboard through the terminal
    helper, verify the panel closes and notices are suppressed without moving
    focus from the helper textarea, then create a transition and verify at most
    the newest queued notice appears after the keyboard closes.
11. Inspect request and browser diagnostics. Message responses must not contain
    endpoint keys, provider session or turn IDs, tmux socket fingerprints,
    tokens, prompts, transcripts, cwd, models, command output, tool arguments,
    or tool output. Invalid cursors return `message_cursor_invalid`; malformed
    read state returns `message_read_state_invalid`; a storage outage returns
    retryable `message_store_unavailable`. Browser notification bodies contain
    only the safe connection label and fixed transition text. If Service Worker
    deduplication storage is unavailable, verify no duplicate notification is
    shown and the durable row remains available.
12. Verify Escape and outside pointer close the panel and return focus to the
    Bell, icon-only controls have accessible names and tooltips, status is not
    communicated by color alone, reduced motion is understandable, delete and
    clear controls are keyboard accessible, and the 320 px viewport has no
    horizontal document overflow. Repeat the row and clear-control assertions
    at phone and tablet widths, where the row delete control must remain
    available without hover.

## Pass gate and cleanup

Correlate every message API response with the state synchronization or UI action
that caused it. Fail on duplicate durable messages, replayed hydration notices,
incorrect shared-target attribution, read-state regression, undeleted rows,
accidental clear without confirmation, wrong unavailable-target navigation,
leaked provider metadata, unexpected browser diagnostics, automatic command
execution, or automatic fullscreen entry. The independent browser notification
and fullscreen controls are covered by their dedicated regression cases.
Capture screenshots of the Bell with an unread badge, the open history panel, a
transient notice, the shared-target `+1` label, the unavailable-target toast,
the clear confirmation, and the keyboard-suppressed phone layout.

Delete only connection instances, tmux sessions, and remote files created by
the case, then reset the disposable fixture home.
