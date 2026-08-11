# PW-SEC-003: HTTPS ingress WebSocket and key-generation path

Priority: P0. Capabilities: core plus a deployed HTTPS Ingress or reverse proxy.
Viewport: desktop. Use a disposable browser context and never print tokens or
private key material.

## Preconditions

1. Confirm the release is deployed and ready. If it is not, follow
   [deployment](../../../docs/deployment.md) and wait for `/healthz` to return
   `200` before continuing.
2. Set `ROAMINAL_E2E_BASE_URL` to the actual HTTPS URL, including a non-default
   port when one is used. The URL must be opened directly; do not use a
   port-forward for this case.
3. Ensure the proxy's HTTP and WebSocket routes point to the same Service and
   preserve the external Host, Upgrade headers, and at least one hour of
   read/send timeout.

## Procedure and assertions

1. Open the HTTPS URL, authenticate, and collect console messages, page errors,
   failed requests, HTTP responses with status `>= 400`, and WebSocket errors
   from the first navigation onward.
2. Open the key manager and generate one supported key. After submission, wait
   for the launch to publish an instance and attach the instance WebSocket.
3. Verify the instance is usable and the browser receives no `403
   origin_denied`, `invalid session id`, early-close handshake error, or
   `WebSocket is closed before the connection is established` message.
4. Verify the browser WebSocket request uses the HTTPS page Origin and the
   matching `wss:` URL. A proxy that labels the transport `wss` in
   `X-Forwarded-Proto` must still allow the HTTPS Origin; no Origin-check bypass
   or wildcard host is acceptable.
5. Delete the generated key and close the disposable connection. Confirm no
   generated private key or authentication token appears in page text, console
   output, screenshots, traces, or test logs.

## Pass gate

The key-generation connection reaches a usable instance over the external
HTTPS endpoint, and all diagnostics are empty except explicitly expected
responses in isolated negative checks. Any unexpected console error/warning,
page error, failed request, HTTP `4xx`/`5xx`, WebSocket error, or leaked secret
fails the case.
