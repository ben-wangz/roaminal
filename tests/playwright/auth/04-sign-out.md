# PW-AUTH-004: Sign out

Priority: P0. Capabilities: core. Viewports: desktop and phone portrait.

## Procedure and assertions

1. Authenticate and open a live local connection. If a tmux pending-launch
   fixture is available, repeat the case while a launch is pending.
2. Click `Sign out` and verify exactly one idempotent logout request is sent.
   The password form replaces the app shell, auth storage is cleared, terminal
   and preview WebSockets close, and a pending launch receives its cancellation
   request.
3. Use browser Back and reload. The protected workspace must not reappear.
4. Log in again and verify the previously live backend connection is still
   listed; signing out revokes browser credentials, not managed terminal
   processes.

## Pass gate

Socket closure caused by sign-out must be clean. In particular, fail on
`WebSocket is closed before the connection is established`, disposed-runtime
exceptions, leaked requests carrying the revoked token, or terminal output
appearing on the login surface.
