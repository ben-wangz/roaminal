# PW-CONN-006: Establish an initial SSH connection

Priority: P0. Capabilities: SSH transport with a dedicated definition and
authorized Ed25519 key. Viewport: desktop.

Fixture: complete [SSH and tmux codespace](../fixtures/ssh-codespace.md) and
use its non-tmux Host values before this case.

## Procedure and assertions

1. Start the SSH definition when no live instance for its alias exists. Verify
   the create request does not include a reuse source and the workspace opens a
   terminal attached to the returned instance ID.
2. If OpenSSH asks for host-key confirmation or a private-key passphrase, answer
   only through the xterm. Roaminal must not display a password/passphrase modal,
   persist the answer, or put it in an HTTP request.
3. Wait for the remote prompt, run commands that print a unique marker,
   `whoami`, `hostname`, and `$SSH_CONNECTION`, and assert output matches the
   fixture without leaking unrelated terminal data.
4. Verify the sidebar transport label is `SSH`, its TARGET/PWD behavior matches
   available metadata, and the card and footer use the SSH Host alias rather
   than the remote hostname. The footer may additionally show only the
   approved safe `user@host:port` endpoint projection.
5. Confirm the connection's WebSocket remains attached while navigating to the
   manager and back. Starting local and switching between local/SSH must route
   keystrokes only to the active runtime.
6. Close the SSH instance and verify the remote shell process ends and the
   instance is retired from the active list.

## Pass gate

Fail on unexpected `ssh transport unavailable`, authentication errors,
host-key/passphrase material in browser storage or diagnostics, duplicate main
WebSockets, or any global diagnostics violation.
