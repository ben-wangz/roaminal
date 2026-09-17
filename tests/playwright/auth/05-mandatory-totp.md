# PW-AUTH-005: Mandatory TOTP enrollment and live reset

Priority: P0. Capabilities: core. Viewport: desktop. Run serially on a
dedicated disposable release: this case deletes and recreates the enrollment
and invalidates every login session.

## Preconditions

- A dedicated Helm release whose state has no `2fa-secret` file (fresh PVC or
  the file removed by the operator). Record `ROAMINAL_E2E_PASSWORD`.
- `kubectl` access to the release Pod for file deletion and inspection; never
  print the secret's contents into the report.
- Standard TOTP computation available locally (six digits, 30-second period,
  SHA-1, one step of tolerance).

## Procedure and assertions

1. Open `/` and sign in with the password. The response returns
   `nextStep: setup_totp` with a pending token; no workspace, connection
   list, terminal, or settings content loads. The pending token used as a
   bearer or refresh credential against `/api/v2/heartbeat` or
   `/api/v2/auth/session` returns 401.
2. The setup screen shows a QR image served from the backend (no external
   QR-generation request), the manual Base32 secret, and a six-digit
   confirmation input. Reload the setup material with the same pending
   token: the same secret and QR are returned. The secret never appears in
   local storage.
3. Confirm with a wrong code: no `2fa-secret` file is created on the PVC.
4. Confirm with the correct code. The response reports
   `reauthenticationRequired`; every browser tab returns to the password
   screen with a success notice, stored auth state is cleared, and there is
   no automatic login. The confirmation code itself is rejected when reused
   for the first full login; the next 30-second code succeeds.
5. Complete the full login and verify the workspace loads. Restart only the
   Pod: the enrollment survives, password-only login stops at
   `verify_totp`, and the previously issued refresh token still rotates
   after restart.
6. While the workspace is open with a live terminal WebSocket, delete
   `<state root>/2fa-secret` via `kubectl exec` without restarting anything.
   Existing streams close within the detection interval, subsequent
   heartbeat and refresh requests fail, and both open tabs return to the
   password screen without showing the workspace.
7. Log out (it must remain usable) and log back in with the password: the
   flow offers `setup_totp` again with a fresh secret. Complete enrollment
   and a full login; connection definitions, instances, and workspace data
   from before the reset are intact.
8. With the backend stopped or via direct file manipulation, replace
   `2fa-secret` with an empty or malformed file and start or exercise the
   backend: login is denied with a retryable storage error and never offers
   enrollment until the file is repaired or removed.

## Pass gate

Run the global diagnostics gate; the expected unauthorized responses from
invalidated credentials must match their exact requests and time window.
Redact the Base32 secret, pending tokens, and six-digit codes from all
artifacts. Screenshots: one of the QR setup stage (secret blurred), one of
the post-deletion login screen, one of the recovered workspace. Restore the
disposable release by completing a fresh enrollment or deleting its PVC;
never touch unrelated releases.
