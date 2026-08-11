# PW-WORK-004: Connected label and instance numbering

Priority: P0. Capabilities: core plus SSH alias. Viewport: desktop.

## Procedure and assertions

1. With one local instance, assert the header is `Connected Local`. With two,
   creation order produces `Local-1` and `Local-2`; there is no space before the
   numeric suffix.
2. With one SSH instance, the header uses the Host alias exactly and never the
   resolved HostName, remote hostname, user, or IP address.
3. Create three instances from the same SSH definition. Switching them produces
   `<alias>-1`, `<alias>-2`, and `<alias>-3` by creation time, not current list
   order or selection order.
4. Create an unrelated alias. It remains unnumbered until that alias has a
   second instance; local and SSH instances never share numbering groups.
5. Close the middle instance and verify surviving suffixes recompute
   deterministically from creation order. Refresh and heartbeat reordering must
   not randomly swap labels.
6. Check long but valid aliases at each standard viewport. The header ellipsizes
   or reflows within its region without overlapping local monitor metrics or
   top actions.

## Pass gate

Capture labels with instance IDs as evidence and run the global diagnostics
gate. No automatic tab switching is allowed during this case.
