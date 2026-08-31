# PW-SEC-001: Browser privacy, asset, and storage boundary

Priority: P0. Capabilities: core. Viewports: desktop and phone portrait.

## Procedure and assertions

1. Start diagnostics before `/` and record every resource URL. Login, manager,
   workspace, Keys, and all icon/font/terminal assets load only from the current
   origin, `data:`, or `blob:`. There are no CDN, analytics, telemetry, or
   third-party requests.
2. Before the user enables System notifications, assert no Service Worker
   registration and no unexpected PWA/background worker. After explicit opt-in,
   exactly one same-origin `/roaminal-sw.js` registration is allowed; no other
   worker is allowed. Reload and verify cache behavior does not serve a stale
   index across a known-version deployment.
3. Inspect localStorage, sessionStorage, IndexedDB, Cache Storage, cookies, URL,
   history entries, DOM attributes, performance entries, and console records.
   Password, proof material, private key bytes, passphrases, raw SSH config,
   remote-monitor collector output, and terminal scrollback must not persist.
4. In the fresh-login phase the intended local storage is auth state and active
   connection selection. After the related feature is explicitly used, the
   allowlist also includes terminal appearance, FileSystem auto-refresh,
   login-session-scoped connection-group collapse, login-session-scoped Virtual
   Keyboard disclosure, and system-notification opt-in. Auth state contains
   tokens/expiry only; token values never appear in URLs, DOM, screenshots,
   response errors, or WebSocket-selected protocol.
5. If notification opt-in is exercised, inspect IndexedDB and verify the only
   allowed database is `roaminal-notification-state`, the only allowed store is
   `shown-messages`, and each record contains only a bounded message ID and
   expiry metadata. Cache Storage, cookies, terminal content, connection
   credentials, notification bodies, and other unexpected persistence remain
   forbidden.
6. Fetch a public key through the explicit Copy action and verify only that
   public material reaches the browser. Inventory/list/edit operations never
   fetch private contents.
7. Exercise a terminal command containing a canary secret, then close and sign
   out. The canary may exist in the protected backend snapshot/audit but must not
   appear in browser persistence or generated test artifacts.

## Pass gate

Use canary hashes when reporting evidence. Any external request, unexpected
storage, secret in diagnostic artifacts, or global diagnostics error fails the
case.
