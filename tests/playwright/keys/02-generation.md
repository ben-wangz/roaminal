# PW-KEY-002: Interactive SSH-key generation

Priority: P0. Capabilities: writable SSH directory with no key of the selected
algorithm. Viewport: desktop. Run serially and restore the fixture afterward.

## Procedure and assertions

1. Open Keys and choose an algorithm that does not exist. The dialog keeps
   Algorithm focused while switching between available algorithms; RSA shows
   2048/3072/4096 bits and defaults to 3072. Filename follows `id_ed25519` or
   `id_rsa`, and comment is optional and bounded.
2. Submit. Exactly one key-generation connection is created and immediately
   opens a valid instance WebSocket; no `invalid session id` response occurs.
3. Complete ssh-keygen prompts through the xterm, testing an empty passphrase in
   the normal regression and an interactive passphrase in a protected run. The
   passphrase must never leave terminal transport or appear in browser storage,
   screenshots, trace attachments, HTTP bodies, or console output.
4. On success, the generated private/public pair is atomically promoted, the
   generation connection exits and retires, the manager/next connection is
   selected according to normal failover, and refresh shows one new inventory
   row with a valid fingerprint.
5. Cancel/fail generation once. No partial destination pair is visible, staging
   data is cleaned, and another generation can start.
6. With an algorithm already present, click Generate for it. The UI rejects the
   action before opening the dialog or creating a connection instance and
   explains that the existing key must be deleted first.
7. Attempt an existing filename or second concurrent generation and verify no
   overwrite occurs.

## Cleanup and pass gate

Delete only the generated pair using the product UI. Fail on focus jumps,
`invalid session id`, stuck `Key generation connection is ready` state, leaked
secrets, orphan staging data, or any global diagnostics violation.
