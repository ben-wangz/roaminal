# PW-CONN-001: Connection-manager navigation and filtering

Priority: P0. Capabilities: core and at least two SSH definitions. Viewports:
desktop, tablet portrait, and phone portrait.

## Procedure and assertions

1. From a workspace click the top-right `Connections` control. The manager
   replaces the terminal workspace; there is no redundant sidebar
   `+ Connections` button and no terminal tab strip.
2. Confirm the Local row is always present and SSH rows show alias,
   `user@host:port`, managed-key count, trust assessment, warning/read-only/tmux
   badges when applicable, and accessible Start/Edit/Duplicate/Delete actions.
3. Filter by an alias substring, hostname substring, and user with mixed case.
   Each query is case-insensitive and whitespace tolerant. The Local row remains
   available while non-matching SSH rows disappear.
4. Use a no-match query and verify the empty state. Clear the query and confirm
   the original order and rows return without a duplicate network mutation.
5. Click Workspace. It opens only when an active instance exists; otherwise the
   manager remains usable and does not render a blank terminal.
6. Exercise the Refresh SSH sources control and verify definitions and keys are
   fetched once each, the busy indicator settles, and no duplicate rows appear.

## Pass gate

Assert no page overflow in each viewport and run the global diagnostics gate.
Filtering must not generate configuration write requests.
