# PW-WORK-004: Connection instance display names

Priority: P0. Capabilities: core plus SSH alias. Viewport: desktop.

## Procedure and assertions

1. With one local instance, assert the card title is `Local`. With two,
   creation order produces `Local-1` and `Local-2`; there is no space before the
   numeric suffix. The active instance footer uses the same resolved display
   name.
2. With one SSH instance, the card and footer use the Host alias exactly and
   never expose the resolved HostName, remote hostname, user, or IP address
   unless the approved safe endpoint projection is intentionally rendered in
   the footer.
3. Create three instances from the same SSH definition. Cards and the active
   footer produce `<alias>-1`, `<alias>-2`, and `<alias>-3` by creation time, not
   current list order or selection order.
4. Create an unrelated alias. It remains unnumbered until that alias has a
   second instance; local and SSH instances never share numbering groups.
5. Close the middle instance and verify surviving suffixes recompute
   deterministically from creation order. Refresh and heartbeat reordering must
   not randomly swap card or footer labels.
6. Check long but valid aliases at each standard viewport. Card and footer text
   ellipsize or reflow within their own regions without overlapping local
   monitor metrics, remote monitor metrics, or top actions. There is no generic
   selected-connection context header.

## Pass gate

Capture labels with instance IDs as evidence and run the global diagnostics
gate. No automatic tab switching is allowed during this case.
