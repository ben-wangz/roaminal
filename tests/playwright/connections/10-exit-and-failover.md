# PW-CONN-010: Shell exit, retirement, and active-view failover

Priority: P0. Capabilities: core; repeat for SSH and tmux when available.
Viewport: desktop. Run serially.

## Procedure and assertions

1. Create connections A, B, and C in that order, then make B active.
2. Type `exit` in B. The terminal becomes terminated, the backend copies the
   final metadata/snapshot to audit material, removes the active-instance data,
   and the card disappears without requiring an intermediate action.
3. The workspace automatically selects C, the next surviving instance in the
   prior order. Its output and input remain usable. No removed runtime may
   reattach or receive input.
4. Make C active and exit it; selection falls back to A. Exit A; with no active
   instances, the UI opens Settings at Connection definitions directly.
5. Repeat by using the action-menu Close command instead of shell `exit` and
   confirm identical retirement/failover semantics.
6. For SSH transport reuse, exit the original instance while a derived instance
   is active and verify the derived connection remains usable and can anchor a
   further Start.
7. For tmux, distinguish closing the SSH/tmux client from killing the tmux
   server: the named session remains attachable unless the test explicitly kills
   it inside tmux.

## Pass gate

There must be no frozen dead terminal, stale card, duplicate history card,
disposed-runtime exception, or unexpected draining transport. Verify audit
material through the isolated test harness without exposing its terminal
contents in reports, then run the global diagnostics gate.
