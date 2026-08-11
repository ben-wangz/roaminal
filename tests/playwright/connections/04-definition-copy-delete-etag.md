# PW-CONN-004: Duplicate, delete, and concurrent ETag conflict

Priority: P1. Capabilities: mutable SSH config and two browser contexts.
Viewport: desktop. Run serially.

## Procedure and assertions

1. Create a source definition with supported fields and tmux options. Duplicate
   it through the row action, provide a unique alias, and verify the copied Host
   block has the same supported SSH values and an independent alias. Verify the
   associated tmux add-on is copied only as defined by current product behavior.
2. Cancel the prompt once. No request or file mutation occurs.
3. Try an invalid or existing alias. The manager shows the server error, keeps
   the original rows intact, and never overwrites a Host block.
4. Open the same source in contexts A and B. Save a change in A, then submit B's
   stale form. B receives an ETag conflict and must refresh before writing; A's
   newer change remains intact.
5. Delete the duplicate, cancel once, then confirm once. The exact Host block and
   stale tmux add-on entry are removed without modifying adjacent raw content.
6. For a read-only or capability-restricted definition, verify duplicate and
   delete controls are disabled and cannot be triggered by forced clicks or
   keyboard activation.

## Cleanup and pass gate

Restore the original fixture and close both contexts. Expected validation and
ETag conflicts are the only allowed failed responses. Run the global browser
diagnostics gate in both contexts.
