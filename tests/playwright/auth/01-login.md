# PW-AUTH-001: Login with mandatory TOTP

Priority: P0. Capabilities: core. Viewports: all five standard projects.

## Preconditions

- Start with a fresh browser context and no Roaminal keys in local storage.
- Register the mandatory diagnostics listeners before navigation.
- The release must have a completed TOTP enrollment. Record the Base32 secret
  through the operator path (`kubectl exec` reading `<state root>/2fa-secret`
  as described in `docs/security.md`), or complete PW-AUTH-005 enrollment
  first and keep its secret. Compute codes locally with standard TOTP
  parameters: six digits, 30-second period, SHA-1, one step of tolerance.

## Procedure and assertions

1. Open `/`. The password field is focused, uses password masking and
   `current-password` autocomplete, and the Continue button is disabled while
   empty. The Roaminal brand and `Secure terminal access` are visible without
   clipping or horizontal page overflow.
2. Submit a deliberately incorrect non-empty password. Correlate one challenge
   request and one failed login request. The UI stays on the password stage,
   shows an accessible error, re-enables submission, and never displays or logs
   the password or proof. No TOTP stage appears for a failed password.
3. Replace it with `ROAMINAL_E2E_PASSWORD` and submit. The login response
   carries `nextStep: verify_totp` and a `pendingToken`, never an access or
   refresh token. The six-digit input is focused, numeric, accepts pasted
   codes, and preserves leading zeros.
4. Enter the current valid TOTP code. The `/auth/2fa/verify` request succeeds,
   the authenticated app shell appears, and the initial view is Settings >
   Connection definitions when no instance is live, or the currently selected
   live workspace.
5. Submit a wrong code once on a fresh login before the correct one. The error
   is shown without leaving the verification stage, and the pending token is
   not reissued; the next correct code still authenticates. Reusing the
   previous login's already-accepted code is rejected and never reveals
   whether it was previously valid.
6. Reload once. Authentication remains valid without showing the password
   form, and no second login challenge is issued.
7. During this fresh-login phase, inspect local storage: only the expected
   Roaminal auth state and active connection selection may exist. No password,
   challenge proof, pending token, TOTP secret or code, private key, terminal
   output, notification opt-in, or other preference may be stored before the
   user explicitly enables that feature.
8. On a separately reset release with a deliberately small `authMaxAttempts`,
   repeat invalid password or TOTP attempts up to the configured limit.
   Challenges and pending tokens are single-use and consumed by failed
   attempts; after the limit, the service returns `service locked` even for a
   correct password plus valid code until the process is restarted. Existing
   terminal/PVC data must not be deleted.
9. In an HTTP browser context that has not been explicitly marked secure,
   submission fails closed with a clear Web Crypto/secure-context error and
   sends no password-derived login proof. The standard HTTPS or narrowly scoped
   test context remains unaffected.

## Pass gate

Run the global diagnostics gate. The expected failed login and failed-code
responses are the only allowed authorization failures and must match their
exact requests. Capture one screenshot of the password stage, one of the code
stage, and one after successful authentication, with all secret-bearing fields
(including the six-digit code) redacted. The lockout variant must use a
disposable release and restore it by restarting only that test workload.
