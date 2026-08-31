# PW-WORK-017: Connection-instance groups

Priority: P1. Capabilities: core, two authenticated browser contexts, and at
least 11 disposable live connection instances. Viewport: desktop, with the
collapse, focus, and overflow assertions repeated at phone width.

## Preconditions

1. Complete the mandatory Helm deployment and browser diagnostics gates in the
   Playwright README. Authenticate two isolated browser contexts as separate
   login sessions and record their initial layouts and connection-instance IDs.
2. Use only connection instances created by this case. Do not mutate the
   connection definition manager or any pre-existing instance. Keep the
   instance IDs and the group layout revision from every response.

## Procedure and assertions

1. Verify the sidebar always renders `Ungrouped`, that new instances append to
   it, and that an empty named group is not created implicitly. The named-group
   limit is 10 members; `Ungrouped` has no member limit.
2. Create a named group through the visible group control. Create a second
   group, then attempt a blank name, a whitespace-padded duplicate, and a
   case-insensitive duplicate. Invalid requests produce the documented error,
   leave the prior layout unchanged, and do not create duplicate groups.
3. Rename a group with surrounding whitespace and verify the persisted name is
   trimmed. Rename it to a case-insensitive duplicate and verify the server
   rejects the mutation with one visible error and the prior name remains.
4. Collapse and expand each group. A collapsed header shows only its name and
   member count, while `Ungrouped` follows the same presentation. Verify
   collapse state is local to the current login session: reload restores it in
   context A, while context B keeps its independent state. Collapse changes do
   not create a server layout revision.
5. Move instances from `Ungrouped` into a named group, between named groups,
   and back. Verify the visible order and server layout use connection-instance
   IDs, not titles or connection definition IDs. Use the per-instance menu and
   group drop target for the same operation and compare the resulting order.
6. Reorder members with pointer drag and with the keyboard reorder control.
   Reorder group headers, including `Ungrouped`, with pointer and keyboard
   input. Search for a title or group name and verify search disables all
   reorder/move mutations; clearing search restores the previous order.
7. Fill a named group to 10 members and attempt an 11th move. Verify the
   request is rejected with the documented capacity error, the optimistic
   layout rolls back exactly once, and no member disappears. Move all members
   to `Ungrouped`, then verify the group becomes empty.
8. Delete the empty group and verify it disappears from the server response
   and after reload. Attempt to delete a non-empty group: the control is
   disabled and a direct request, if attempted through an isolated API
   context, returns the documented non-empty-group error without mutation.
9. Start another disposable connection instance after groups exist. Verify it
   is appended to `Ungrouped` regardless of the source definition or the last
   selected group. Retire one grouped instance and verify survivors retain
   their relative order; no automatic reordering or empty-group deletion is
   invented.
10. In context A, hold a stale layout revision in a second page/context. Save
    a conflicting layout from context B, then submit the stale mutation from A.
    Verify the server returns the documented revision conflict, the UI rolls
    back its optimistic layout, shows one error, and performs at most one safe
    refresh/retry. A repeated conflict must not duplicate a mutation or lose
    the winning layout.
11. Repeat an empty-group delete while the first delete is delayed. Verify the
    control becomes busy, only one DELETE is sent, the final empty group is
    absent, and a retry after a transient failure sends one new request only.
12. Reload both contexts and verify the server-persisted group order, names,
    and membership are identical to the winning revision. Change the layout in
    context A and verify context B adopts it on its next authoritative refresh,
    while each context retains its own collapse state. No credentials,
    endpoint keys, tmux data, or terminal output may appear in layout requests.

## Pass gate and cleanup

Correlate every create, rename, layout, move, reorder, and delete response with
the visible action. Fail on duplicate requests, stale optimistic state,
silent capacity overflow, deletion of a non-empty group, cross-login collapse
state, unexpected diagnostics, or any horizontal overflow. Capture the named
group, collapsed layout, capacity rejection, conflict rollback, and final
mobile screenshots.

Delete only the groups and connection instances created by this case. Restore
both login-session local collapse keys and close every disposable context.
