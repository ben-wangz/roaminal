# PW-WS-016: Browser fullscreen and system notifications

Priority: P1. Capabilities: core. Viewports: desktop, tablet portrait, phone
portrait, and phone landscape. Use a fresh browser context for permission and
fullscreen assertions.

## Procedure and assertions

1. Run the mandatory Helm deployment and browser diagnostics gates from the
   Playwright README. Register permission, fullscreen, Service Worker, console,
   page-error, request-failure, response, and WebSocket diagnostics before the
   first navigation.
2. On every app page with a topbar, verify one `fullscreen-toggle` control is
   present with an accessible label and maximize icon when inactive. If runtime
   capability detection reports unsupported, the control is disabled with a
   clear label and login, connection manager, Terminal, FileSystem, messages,
   and input remain usable.
3. In a Chromium context where element fullscreen is permitted, enter and exit
   fullscreen through a real click. Verify the target is the complete
   `.app-shell`, the topbar, unified workspace tool surface, Monitor,
   Terminal/FileSystem body, and Message Center remain usable, and the control
   changes to the minimize state. Press Escape or trigger a native external
   exit and verify the control synchronizes without an automatic re-entry.
4. Exercise fullscreen on Terminal and FileSystem, switch connection instances,
   open/collapse Connections and Virtual keyboard, and use Terminal input. No
   mode change, connection change, native keyboard opening, timer, or FileSystem
   preview action may request fullscreen. On phone layouts verify safe-area
   spacing and an in-app exit affordance when the normal topbar is hidden by the
   native keyboard.
5. Open Appearance and inspect System notifications. Before an explicit Enable
   click, no permission prompt or Service Worker registration is created. Click
   Enable in a secure context, grant permission, and verify the state reflects
   the granted browser permission. When the authenticated backend config
   reports Web Push enabled, verify the page requests the config and registers
   exactly one subscription without exposing its endpoint or keys. When the
   backend reports `enabled: false`, verify the UI remains Foreground only and
   does not attempt subscription registration. A denied permission produces a
   blocked state and does not repeatedly prompt; an insecure or unsupported
   context produces Unavailable while the Message Center remains usable.
6. With notifications enabled and the page hidden or unfocused, create one
   eligible `codex_turn_completed` and one `codex_turn_failed` message. Verify
   one browser notification per durable `messageId`, safe title/body, and only
   the message ID in notification data. `agent_reporting_ready` does not create
   a browser notification. Replayed heartbeat/list responses and two open tabs
   do not duplicate it; visible foreground delivery is suppressed.
7. Click a notification and verify the Service Worker focuses or opens the
   same-origin application and forwards only the message ID. A live target
   selects that connection's Terminal workspace and marks the message read. A
   historical target leaves the current connection/workspace unchanged and
   shows exactly `The connection for this message is no longer connected.`.
8. Delete one Message Center row and clear all messages. Matching browser
   notifications close when the browser exposes notification enumeration;
   cleanup failure does not fail the durable mutation. Toggle System
   notifications off and verify local notification delivery stops and the
   authenticated backend receives one delete-all subscription request. Native
   keyboard opening suppresses in-page notices and does not steal focus from
   the terminal helper.

## Pass gate

Fail on permission prompts without a user click, leaked endpoint/event/prompt/
transcript/cwd/model/token data, duplicate notifications, automatic fullscreen
entry, new-window behavior outside notification click handling, unexpected
browser diagnostics, or any protected API request not caused by the action under
test. OS tray, sound, vibration, lock-screen rendering, and iOS delivery are
not proven by Playwright and require the manual device smoke checks in the
browser notification/fullscreen plans.
