# PW-REL-006: client diagnostics preserve Console output and report failures

Priority: P1. Capabilities: core Helm deployment, Pod log access, and a
dedicated disposable release/PVC. Viewport: desktop. Use a unique marker and
never use real credentials or private key material.

## Preconditions

1. Run the mandatory Helm deployment gate in
   [`tests/playwright/README.md`](../README.md). If the release is missing or
   unready, follow the Helm deployment procedure in
   [`docs/deployment.md`](../../../docs/deployment.md) before starting
   Playwright. Do not use a port-forward.
2. Use a disposable release with `app.clientDiagnosticsEnabled=true`, direct
   Service/Ingress access, Pod log access, and a writable unified PVC. Keep the
   release isolated from other users because this case intentionally writes
   diagnostics records.
3. Attach `page.on('console')`, `page.on('pageerror')`,
   `page.on('requestfailed')`, `page.on('response')`, and
   `page.on('websocket')` before the first `page.goto()`. Retain complete
   output through cleanup.
4. Authenticate through the visible login UI. Record the release version and
   `bootId` from `/api/version` without recording access or refresh tokens.

## Procedure

1. Generate a marker containing only a timestamp and random UUID. Evaluate
   `console.error(marker, { value: 'diagnostic-test-object' })`. Assert that
   Playwright receives exactly one `error` Console message with the marker and
   that the original object argument remains visible through Console argument
   inspection. This intentional error is the only expected Console error in
   this step.
2. Evaluate an unhandled rejection with `new Error(marker + '-rejection')` and
   add a broken same-origin image whose URL contains no query or secret. Assert
   the expected `pageerror`/resource failure and retain the browser evidence.
3. Start one disposable connection, then use the browser network interception
   API to abort exactly one product WebSocket attempt before it opens. Assert
   one `websocket_error` diagnostic later, with endpoint kind, phase, and
   connection instance ID. The browser diagnostic may say only that the socket
   failed before open; it must not claim an HTTP status unavailable to the
   WebSocket API.
4. In one deliberate Console call, include the following fake values:
   `Bearer test-token`, `accessToken=test-access`,
   `https://example.invalid/path?secret=test`, and a PEM private-key marker.
   Assert that Playwright still shows the original call, while server records
   contain none of the fake token, query value, or private-key body.
5. Repeat the same marker error several times within 30 seconds. Assert that
   the server receives one event with a repeat count rather than one record per
   call. Temporarily fail the diagnostics POST route and assert that the
   application remains usable, no recursive diagnostics storm appears, and the
   bounded retry behavior eventually stops.
6. Inspect the Pod log for `client_diagnostic=` records and inspect the private
   state path with a read-only command. Locate the matching `pageId` and
   `eventId`, verify the runtime version, boot ID, auth session, event kinds,
   and redaction. Do not print the complete file in the test report.
7. Deploy a second disposable revision with
   `app.clientDiagnosticsEnabled=false`. Repeat the explicit Console error and
   `GET /api/version`; assert the capability is false, the diagnostics route is
   unavailable, Console output is unchanged, and no new record is appended.
8. Remove only the disposable revisions and connection instances created by
   this case. Retain failed-run logs and traces as evidence, subject to secret
   review.

## Assertions and failure rules

- The original Console message remains visible once, with type `error`; the
  feature must not suppress, rewrite, or downgrade it.
- Every expected intentional browser error is matched by its unique marker.
  Any unrelated Console `error` or `warning`, page error, failed request,
  unexpected HTTP `4xx`/`5xx`, or WebSocket error fails the case.
- No access token, refresh token, subprotocol, fake private key, terminal input,
  terminal output, SSH configuration, host alias, PWD, or command text appears
  in page text, Console output beyond the deliberate fake call, network URLs,
  Pod logs, NDJSON records, screenshots, traces, or reports.
- The diagnostics endpoint is same-origin and authenticated. An unauthenticated
  POST returns `401` and does not create a record.
- Records are single-line JSON, deduplicated, and bounded. A persistence
  failure does not make the application unavailable.
- After all assertions, run the complete browser diagnostics gate from the
  Playwright README and explicitly review the full Console output. Passing
  screenshots without this review is not a pass.

## Evidence

Record `PASS`, `FAIL`, or `SKIPPED` with the deployment-gate record, release
version, boot ID, viewport, marker hash (not the marker if it could be reused),
browser diagnostics, API/WebSocket observations, and redacted log excerpts.
If Pod access, a writable disposable release, or a required browser network
interception capability is unavailable, report `BLOCKED` or `SKIPPED` with the
exact missing prerequisite; never silently omit the case.
