# PW-KEY-001: SSH-key inventory and public-key copy

Priority: P0. Capabilities: SSH directory with controlled key fixtures and
clipboard permission. Viewport: desktop. Run serially.

## Fixture matrix

Provide valid `id_ed25519`, a custom `*_ed25519`, `id_rsa`, a custom `*_rsa`,
their public files where applicable, an unsupported algorithm/name, a broken
file, a safe read-only projected symlink within the SSH root, and an escaping
symlink.

## Procedure and assertions

1. Open Keys and refresh. Only allowlisted Ed25519/RSA private-key names appear.
   Each row accurately reports filename, algorithm/bits, fingerprint,
   availability, writable/read-only status, and public-key availability.
2. Unsupported names, public-only files, broken material, and escaping symlinks
   are not presented as usable private keys and never cause the whole panel to
   fail.
3. Copy an available public key. The clipboard contains exactly one normalized
   public-key line for that key; no private bytes are fetched or copied. A
   success toast appears and the button leaves its busy state.
4. Deny clipboard permission and repeat. The UI reports failure without losing
   inventory state or logging the public key.
5. Verify the page and network bodies contain metadata and the selected public
   key only when explicitly requested; private key endpoints/content do not
   exist.

## Pass gate

Redact clipboard contents from artifacts. Run the global diagnostics gate;
expected clipboard denial is handled UI state, not an uncaught rejection.
