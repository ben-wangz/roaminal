# PW-CONN-008: Tmux connection creation and attachment

Priority: P0. Capabilities: SSH transport plus tmux. Viewports: desktop and
phone portrait. Run serially.

Fixture: complete [SSH and tmux codespace](../fixtures/ssh-codespace.md),
including remote-state preparation, before this case.

## Preconditions

- The definition has tmux enabled in the Roaminal YAML add-on and a unique valid
  session name.
- Test both a missing session and a pre-existing session owned by the fixture.

## Procedure and assertions

1. Start with no named session. The UI opens a pending launch runtime, the
   backend preflights `command -v tmux` and `tmux ls`, creates the named session,
   publishes one live instance, and replaces the launch WebSocket with the
   instance WebSocket without freezing or rendering a disposed runtime.
2. Write a unique marker, detach or close the visible connection as prescribed
   by the fixture, then Start the same definition again. It attaches the same
   named tmux session and the marker/history is visible.
3. With one live instance, press Start repeatedly and verify each request opens
   another channel over the existing SSH transport while all instances point to
   the configured tmux session. Closing one view must not kill tmux or the other
   views.
4. Verify the Virtual Keyboard defaults to Common mode. Select Tmux manually
   and verify its prefix is the effective remote server prefix (`C-k` from the
   remote `~/.tmux.conf`, native `C-b`, no-config fallback `C-b`, or unsupported
   state), not a parse of the local Roaminal config.
5. Verify Esc, prefix+`o` (next pane), prefix+`d` (detach), and prefix+`"`
   (horizontal split) send the exact sequences for the effective prefix. A
   custom prefix must change these sequences together with the displayed labels.
6. Resize both desktop and phone terminals, run `stty size` inside tmux, and
   assert it converges to the active xterm rows/columns. Reattach and repeat to
   catch stale tmux client sizing.
7. Against an SSH fixture without `tmux`, Start must fail before publishing a
   connection, show a concise error, and leave no pending or active instance.

## Pass gate

Fail on `terminal runtime ... is disposed`, early-closed WebSocket messages,
duplicate pending sessions, blank/frozen xterm, or an unexpected transport
fallback. Run the global diagnostics gate after every launch transition.
