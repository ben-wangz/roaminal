# PW-WORK-016: Browser fullscreen and system notifications

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
   capability detection reports unsupported, the control remains visible,
   carries `data-fullscreen-state="unsupported"`, is disabled, has an
   unavailable visual marker and clear label, and login, Settings, Terminal,
   Files, messages, and input remain usable. Verify that a
   disabled control does not issue a fullscreen request.
3. In a Chromium context where element fullscreen is permitted, enter and exit
   fullscreen through a real click. Verify the target is the complete
   `.app-shell`, the topbar, unified workspace tool surface, Monitor,
   Terminal/File preview body, and Message Center remain usable, and the control
   changes to the minimize state. Press Escape or trigger a native external
   exit and verify the control synchronizes without an automatic re-entry. A
   rejected request or fullscreen error must clear the pending state and return
   the control to its supported or unsupported state.
4. Exercise fullscreen on Terminal and File preview, switch connection instances,
   open/collapse Connections and Virtual keyboard, and use Terminal input. No
   content change, connection change, native keyboard opening, timer, or Files
   preview action may request fullscreen. On phone layouts verify safe-area
   spacing and an in-app exit affordance when the normal topbar is hidden by the
   native keyboard. In an iPhone-sized emulated context, verify visibility and
   runtime-driven state only; desktop Chromium emulation must not be reported
   as proof of iPhone WebKit support.
5. Open Settings > Notifications and inspect System notifications. Before an explicit Enable
   click, no permission prompt or Service Worker registration is created. Click
   Enable in a secure context, grant permission, and verify the state reflects
   the granted browser permission. When the authenticated backend config
   reports Web Push enabled, verify the page requests the config and registers
   exactly one subscription without exposing its endpoint or keys. When the
   backend reports `enabled: false`, verify the UI remains Foreground only and
   does not attempt subscription registration. A denied permission produces a
   blocked state and does not repeatedly prompt; an insecure or unsupported
   context produces Unavailable while the Message Center remains usable.
6. With notifications enabled and the page hidden or unfocused, use the
   preference controls covered in `PW-WORK-018` and create one
   `running -> relax` transition. Verify one browser notification per durable
   `messageId`, safe
   title/body, and only the message ID in notification data. Create a normal
   `relax -> running` transition and verify it is shown in Message Center but
   does not create a browser notification. Create `running -> error` only with
   a provider fixture that explicitly supports the error capability; the
   current Codex provider must not infer it. Replayed heartbeat/list responses
   and two open tabs do not duplicate notifications; visible foreground
   delivery is suppressed.
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
