# PW-AUTH-001: Login

Priority: P0. Capabilities: core. Viewports: all five standard projects.

## Preconditions

- Start with a fresh browser context and no Roaminal keys in local storage.
- Register the mandatory diagnostics listeners before navigation.

## Procedure and assertions

1. Open `/`. The password field is focused, uses password masking and
   `current-password` autocomplete, and the Connect button is disabled while
   empty. The Roaminal brand and `Secure terminal access` are visible without
   clipping or horizontal page overflow.
2. Submit a deliberately incorrect non-empty password. Correlate one challenge
   request and one failed login request. The UI stays on the login surface,
   shows an accessible error, re-enables submission, and never displays or logs
   the password or proof.
3. Replace it with `ROAMINAL_E2E_PASSWORD` and submit. The challenge and login
   requests succeed, the authenticated app shell appears, and the initial view
   is either the connection manager or the currently selected live workspace.
4. Reload once. Authentication remains valid without showing the password form,
   and no second login challenge is issued.
5. During this fresh-login phase, inspect local storage: only the expected
   Roaminal auth state and active connection selection may exist. No password,
   challenge proof, private key, terminal output, notification opt-in, or other
   preference may be stored before the user explicitly enables that feature.
6. On a separately reset release with a deliberately small
   `authMaxAttempts`, repeat invalid logins up to the configured limit. Each
   challenge is single-use and consumed by a failed attempt; after the limit,
   the service returns `service locked` even for a correct password until the
   process is restarted. Existing terminal/PVC data must not be deleted.
7. In an HTTP browser context that has not been explicitly marked secure,
   submission fails closed with a clear Web Crypto/secure-context error and
   sends no password-derived login proof. The standard HTTPS or narrowly scoped
   test context remains unaffected.

## Pass gate

Run the global diagnostics gate. The expected failed login response is the only
allowed authorization failure and must match its exact request. Capture one
screenshot of the error state and one after successful authentication, with all
secret-bearing fields redacted. The lockout variant must use a disposable
release and restore it by restarting only that test workload.
