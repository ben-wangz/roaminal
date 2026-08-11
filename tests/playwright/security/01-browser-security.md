# PW-SEC-001: Browser privacy, asset, and storage boundary

Priority: P0. Capabilities: core. Viewports: desktop and phone portrait.

## Procedure and assertions

1. Start diagnostics before `/` and record every resource URL. Login, manager,
   workspace, Keys, and all icon/font/terminal assets load only from the current
   origin, `data:`, or `blob:`. There are no CDN, analytics, telemetry, or
   third-party requests.
2. Assert no service worker registrations and no unexpected PWA/background
   workers. Reload and verify cache behavior does not serve a stale index across
   a known-version deployment.
3. Inspect localStorage, sessionStorage, IndexedDB, Cache Storage, cookies, URL,
   history entries, DOM attributes, performance entries, and console records.
   Password, proof material, private key bytes, passphrases, raw SSH config,
   remote-monitor collector output, and terminal scrollback must not persist.
4. The only intended local storage is auth state and active connection
   selection. Auth state contains tokens/expiry only; token values never appear
   in URLs, DOM, screenshots, response errors, or WebSocket-selected protocol.
5. Fetch a public key through the explicit Copy action and verify only that
   public material reaches the browser. Inventory/list/edit operations never
   fetch private contents.
6. Exercise a terminal command containing a canary secret, then close and sign
   out. The canary may exist in the protected backend snapshot/audit but must not
   appear in browser persistence or generated test artifacts.

## Pass gate

Use canary hashes when reporting evidence. Any external request, unexpected
storage, secret in diagnostic artifacts, or global diagnostics error fails the
case.
