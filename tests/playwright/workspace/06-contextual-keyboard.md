# PW-WORK-006: Contextual Tmux and Codex virtual keyboard

Priority: P0. Capabilities: core; tmux fixture for prefix variants. Viewport:
desktop. Run serially for byte-capture fixtures.

## Procedure and assertions

1. Every local/SSH instance defaults to Common, regardless of `tmuxEnabled`.
   Manual mode choices are isolated per connection instance for the page
   lifetime and reset to Common after reload. The Virtual Keyboard is opened
   from the shared Connections/Virtual keyboard selector. On desktop it
   replaces the connection surface as a left panel; on tablet and phone it is
   placed below the Terminal workspace.
   Common, Tmux, and Codex are peer modes, and only the selected mode is
   rendered.
2. Select Common and use a terminal byte-capture command/fixture.
  Assert exact bytes for Esc `0x1b`, Tab `0x09`, Enter `0x0d`, Ctrl+C `0x03`,
   `|`, `~`, `/`, and the four arrow sequences. The four visible direction
   labels are `↑`, `↓`, `←`, and `→`; their accessible names remain descriptive.
   Verify the Paste button is present and reads clipboard text into the active
   terminal exactly once. The Common section is rendered exactly once. Measure
   every visible button and verify its label is fully readable, with no clipping
   or horizontal overflow at 320px.
3. Press Codex-specific keys and assert exact bytes: Ctrl+T `0x14`, PageUp
   `ESC [ 5 ~`, PageDown `ESC [ 6 ~`, and `q`. Press `commit and push`, `/model`,
   and `/compact`; each enters literal ASCII text with no newline/Enter and
   performs no Git, shell-command, or external API action until the user
   explicitly submits it. Codex contains no duplicate Escape or Enter key.
4. For a remote effective prefix `C-k` (configured in the remote
   `~/.tmux.conf`), Tmux buttons display `Ctrl+K` and `Ctrl+K [` and send
   `0x0b` and `0x0b 0x5b`. Repeat native `C-b` and no-config fallback `C-b`
   metadata; both send `0x02` and `0x02 0x5b`. Label and bytes must always come
   from the same model.
5. Tmux mode also provides prefix+`o` (next pane), prefix+`d` (detach), and
   prefix+`"` (horizontal split). With effective `C-k`, the
   latter buttons display `Ctrl+K o`, `Ctrl+K d`, and `Ctrl+K "` and send
   `0x0b 0x6f`, `0x0b 0x64`, and `0x0b 0x22`; native/fallback `C-b` uses the
   same suffixes after `0x02`.
6. With `tmuxPrefixSource=unsupported`, prefix-dependent buttons (prefix,
   copy-mode, `o`, `d`, and `"`) are disabled with an explanatory tooltip;
   Common keys, PageUp, PageDown, and q remain usable.
7. The shared selector opens Connections or Virtual keyboard without changing
   the active connection. They are never simultaneously expanded. The common
   surface collapse control closes the selected tool; switching to FileSystem
   selects Connections and hides the keyboard surface. The login-scoped
   keyboard preference is restored after a native keyboard dismissal when the
   keyboard tool was selected.
8. When the native mobile keyboard is open, the Virtual Keyboard key content is
   hidden and the active xterm helper textarea is the only text-entry path;
   no application-owned composer or Send control is rendered. Closing the
   native keyboard restores the saved Virtual Keyboard preference.
9. During a pending launch, before WebSocket connection, after disconnection,
   and after exit, every applicable key is disabled and sends no frames to an
   old runtime. Reconnection enables them only for the current live instance.
10. Every click returns focus to the active xterm without switching cards or
   resizing the layout.

## Pass gate

Record hex byte assertions, not secret terminal content. Run the global
diagnostics gate and fail any cross-instance input or unintended command
execution. Capture the computed key dimensions and the visible arrow labels.
