# PW-WORK-006: Contextual Tmux and Codex virtual keyboard

Priority: P0. Capabilities: core; tmux fixture for prefix variants. Viewport:
desktop. Run serially for byte-capture fixtures.

## Procedure and assertions

1. A normal local/SSH instance defaults to Codex; a `tmuxEnabled=true` instance
   defaults to Tmux. Manual mode choices are isolated per instance for the page
   lifetime and reset to defaults after reload.
2. Use a terminal byte-capture command/fixture and press Codex keys. Assert exact
   bytes: Ctrl+T `0x14`, PageUp `ESC [ 5 ~`, PageDown `ESC [ 6 ~`, Esc `0x1b`,
   and `q`. No key appends Enter.
3. Press `commit and push`. It has a visible text icon, enters exactly that
   literal ASCII text with no newline/Enter, and performs no Git, shell-command,
   or external API action until the user explicitly submits it.
4. For a remote effective prefix `C-k` (configured in the remote
   `~/.tmux.conf`), Tmux buttons display `Ctrl+K` and `Ctrl+K [` and send
   `0x0b` and `0x0b 0x5b`. Repeat native `C-b` and no-config fallback `C-b`
   metadata; both send `0x02` and `0x02 0x5b`. Label and bytes must always come
   from the same model.
5. Tmux mode also provides Esc (`0x1b`), prefix+`o` (next pane), prefix+`d`
   (detach), and prefix+`"` (horizontal split). With effective `C-k`, the
   latter buttons display `Ctrl+K o`, `Ctrl+K d`, and `Ctrl+K "` and send
   `0x0b 0x6f`, `0x0b 0x64`, and `0x0b 0x22`; native/fallback `C-b` uses the
   same suffixes after `0x02`.
6. With `tmuxPrefixSource=unsupported`, prefix-dependent buttons (prefix,
   copy-mode, `o`, `d`, and `"`) are disabled with an explanatory tooltip; Esc,
   PageUp, PageDown, and q remain usable.
7. During a pending launch, before WebSocket connection, after disconnection,
   and after exit, every applicable key is disabled and sends no frames to an
   old runtime. Reconnection enables them only for the current live instance.
8. Every click returns focus to the active xterm without switching cards or
   resizing the layout.

## Pass gate

Record hex byte assertions, not secret terminal content. Run the global
diagnostics gate and fail any cross-instance input or unintended command
execution.
