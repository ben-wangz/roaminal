# PW-KEY-003: Delete writable keys and protect mounted keys

Priority: P0. Capabilities: one disposable writable pair and one read-only
mounted/projected pair. Viewport: desktop. Run serially.

## Procedure and assertions

1. Cancel deletion of the writable key. No DELETE request occurs and both files
   remain.
2. Confirm deletion. The button is busy during the request, the private and
   matching public file are removed as one managed pair, the row disappears,
   and unrelated files are untouched.
3. Refresh and verify the deleted key does not reappear. A connection definition
   that referenced it remains readable but reports the missing/unmanaged
   identity condition rather than exposing stale key material.
4. For the read-only mounted/symlink key, the delete control is disabled with
   `Mounted key cannot be deleted`. Forced DOM activation and a direct DELETE
   request both fail closed and the mounted files remain intact.
5. Verify deleting a key never deletes an SSH Host definition or a running
   connection instance.

## Cleanup and pass gate

Restore the disposable fixture outside product storage only after Roaminal has
finished the test. The expected direct API denial is the only failed response;
run the global diagnostics gate.
