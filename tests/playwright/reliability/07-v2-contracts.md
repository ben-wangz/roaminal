# PW-REL-007: 0.3 HTTP and WebSocket contracts

This case verifies the public 0.3 transport boundary after the mandatory Helm
deployment gate in `playwright/README.md` has passed. Use the dedicated release,
the password from `ROAMINAL_E2E_PASSWORD`, and the standard diagnostics
listeners before the first navigation.

## Version boundary

1. Navigate to `ROAMINAL_E2E_BASE_URL` through the visible login flow.
2. Request `GET /api/v2/version` from the page context. Assert `200`,
   `name=roaminal`, `apiVersion=roaminal.v2`, the deployed product version, and
   a non-empty `bootId`.
3. Request the retired `GET /api/version` route. This is an expected negative
   response: assert exactly `404` and a static or empty body, and record it as
   an intentional retired-route check rather than a failed request.
4. Confirm that the browser has made no `/api/` request other than `/api/v2/`
   during the case.

## Structured errors

1. Send an unauthenticated request to
   `GET /api/v2/connection-instances`. Assert exactly `401` and a JSON body
   containing string `error`, stable string `code=unauthorized`, and boolean
   `retryable`.
2. After authentication, send an invalid JSON mutation containing an unknown
   field to `PUT /api/v2/connection-instances/order`. Assert exactly `400` and
   the same error envelope shape. Do not accept an obsolete two-field error body.
3. Verify that no access token, refresh token, terminal output, or password is
   present in the request/response diagnostics or trace.

## Terminal roles and cleanup

1. Create or select a disposable local connection instance and wait for its
   terminal WebSocket. Assert the URL uses
   `/ws/v2/connection-instances/<connectionInstanceId>` and the negotiated
   protocol is `roaminal.v2`.
2. Verify that the interactive terminal can send a harmless input and receive
   the typed initial order `snapshot`, `meta`, `status`, followed by output.
3. Open the sidebar preview for the same instance. Assert that its WebSocket
   URL adds `?role=observer` and remains on `/ws/v2/`. The preview receives
   snapshot/output data but does not send input, resize, or control-claim
   commands.
4. Attempting a control command from an observer socket is an expected
   negative check: assert close code `1008` and reason
   `observer_cannot_control` if the browser harness can issue the command.
5. Dispose the preview, close the terminal, delete only the disposable local
   connection instance, and verify all expected WebSockets close without a
   diagnostics error.

## Evidence and pass criteria

Retain the Helm gate record, version response, exact expected negative response
bodies, WebSocket URL/protocol/close records, final URL, screenshot, and
cleanup result. The case passes only when all assertions and the mandatory
diagnostics gate are clean. A missing Helm release or unavailable required
credential is `BLOCKED`, not a product pass.
