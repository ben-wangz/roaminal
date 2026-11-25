# PW-AUTH-005: Mandatory TOTP enrollment and offline reset

Priority: P0. Capabilities: core and offline maintenance. Viewport: desktop.
Run serially on a dedicated disposable release. Reset invalidates every login
session.

## Preconditions

- A dedicated Helm release with an unenrolled database, either on a fresh PVC
  or after [offline TOTP reset](../../../docs/security.md#resetting-totp-enrollment).
  Keep the release password and test enrollment secret in private fixture storage.
- Authorization to stop, restart, and run maintenance for this release only.
  Use the same runtime image, state mount, and database env as the Deployment.
- Local TOTP computation: six digits, 30-second period, SHA-1, one step of tolerance.

## Procedure and assertions

1. Open `/` and sign in with the password. The response returns
   `nextStep: setup_totp` with a pending token; no workspace, connection
   list, terminal, or settings content loads. The pending token used as a
   bearer or refresh credential against `/api/v2/heartbeat` or
   `/api/v2/auth/session` returns `401`.
2. The setup screen shows a QR image served from the backend (no external
   QR-generation request), the manual Base32 secret, and a six-digit
   confirmation input. Reload the setup material with the same pending
   token: the same secret and QR are returned. The secret never appears in
   local storage.
3. Confirm with a wrong code. The confirmation fails; a fresh password
   sign-in still offers `setup_totp` rather than `verify_totp`. Fetch setup
   material for the new pending token before continuing.
4. Confirm with the correct code. The response reports
   `reauthenticationRequired`; every browser tab returns to the password
   screen with a success notice, stored auth state is cleared, and there is
   no automatic login. The confirmation code itself is rejected when reused
   for the first full login; the next 30-second code succeeds.
5. Complete the full login and verify the workspace loads. Restart only the
   release Pod and rerun the Helm gate. Password-only login stops at
   `verify_totp`, and the previously issued refresh token still rotates
   after restart. Save the current refresh token privately for the reset check.
6. Use [offline maintenance](../../../docs/deployment.md#offline-maintenance) to
   stop the release, wait for its Pod to exit, then run
   `roaminal admin totp reset --confirm`. Resume serving with the same database
   and rerun the Helm gate before opening a browser. Do not modify DB rows or
   delete any files. The pre-reset refresh token returns `401`.
7. Sign in with the password: the flow offers `setup_totp` again with a fresh
   secret. Complete enrollment and a full login. Verify existing connection
   definitions and workspace files are still present. Live connections do not
   resume after the Pod restart, and a new login has its own workspace layout.

## Pass gate

Run the global diagnostics gate; match expected failed authentication and
WebSocket closures to the exact requests or planned restarts. Redact the Base32
secret, pending tokens, and six-digit codes from all artifacts. Restore the
fixture with a fresh test enrollment and remove any maintenance Pod, even on
failure. Never touch unrelated releases.
