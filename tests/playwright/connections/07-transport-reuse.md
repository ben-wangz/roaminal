# PW-CONN-007: Reuse an existing SSH transport

Priority: P0. Capabilities: SSH transport. Viewport: desktop. Run serially.

## Procedure and assertions

1. Start instance A for the test alias and prove it is usable. Return to the
   manager. The row still says `Start`; there is no accumulating `Open over
   existing transport` button.
2. Click Start again. Verify the create request names A as
   `reuseFromConnectionInstanceId`, instance B has a different UUID, and B opens
   a new remote channel over the same ControlMaster. Both sidebar cards remain
   independently selectable and usable.
3. Start C from the same row and verify the manager chooses one current live
   instance as the hidden anchor instead of rendering one button per instance.
4. Close original owner A while B and C remain live. Start D from the manager.
   D must connect successfully over the surviving transport; the transport must
   not become draining merely because its first visible instance exited.
5. Close B and verify C and D remain interactive. Close channels until none
   remain, then Start again and verify a fresh initial transport can be created.
6. Ensure a local, exited, source-changed, deleted, or unrelated alias instance
   is never selected as a reuse anchor.

## Pass gate

The expected path contains no `ssh transport is draining` or `ssh transport
unavailable`, no extra Start/reuse controls, and no output crossed between
instances. Record instance IDs and reuse relationships, not socket paths or
credentials, then run the global diagnostics gate.
