# PW-WORK-001: Terminal input, output, focus, and shell metadata

Priority: P0. Capabilities: core. Viewports: all five standard projects.

## Procedure and assertions

1. Open a local connection and wait for exactly one `.xterm-screen` with a
   non-empty snapshot/prompt. Click the viewport and type a unique `printf`
   command. Input echo and command output appear in order and exactly once.
2. Print UTF-8 text split across writes, ANSI colors, a long line, and enough
   lines to enter scrollback. Text remains valid, control sequences render
   instead of leaking, horizontal page overflow does not appear, and scrollback
   remains searchable.
3. Run `cd` to a unique directory and set a shell title. Wait for heartbeat/meta
   updates; the footer PWD, document title, and automatic card title reflect
   the new state without private marker bytes appearing in xterm output. The
   card keeps PWD only in its accessible detail path, while the footer exposes
   the active Terminal state, connection name, safe endpoint, TERM, current
   `COLS x ROWS`, and `tmux`/`ssh`/`local` context without stale values.
4. Start a long command that emits Roaminal execution markers. The status area
   reports Running and then clears/shows completion once. Normal commands that
   do not use the shell integration remain usable.
5. In Settings > Notifications, verify the System notifications setting is independent from
   terminal command completion. Completing an integrated command still shows
   the in-app `Command completed` toast, but it does not create a browser
   notification. Browser notifications are reserved for the configured Agent
   state transitions covered by the Agent notification regression case.
6. Create a second connection. Rapidly alternate focus and send unique markers
   to each; no input or output crosses runtimes, and only one main xterm is
   mounted at a time.
7. Verify browser paste of non-secret multiline text preserves bytes and does
   not trigger duplicate command submission.
8. With a Chinese IME active, enter fullwidth punctuation such as `，。！？；：`
   in the terminal. The resulting shell output preserves those exact characters
   once, including when the textarea commits at keyup; normal ASCII punctuation
   remains usable. While composing a Chinese word, intermediate candidate text
   must remain in the IME composition view and must not be sent before the final
   committed character.

## Pass gate

Fail on malformed UTF-8, raw private markers, missing/duplicated output,
`onShowLinkUnderline`, disposed-runtime errors, or unexpected socket reconnects.
Run the global diagnostics gate and redact terminal contents if they may contain
sensitive environment data.
