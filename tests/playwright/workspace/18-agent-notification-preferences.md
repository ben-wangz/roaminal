# PW-WORK-018: Agent notification preferences

Priority: P1. Capabilities: core, an SSH/tmux Agent fixture, browser
notifications, and two authenticated login sessions. Viewports: desktop and
phone portrait.

## Preconditions

1. Complete the mandatory Helm deployment and browser diagnostics gates in the
   Playwright README. Use a supported SSH/tmux connection definition and two
   connection instances that share its tmux session target. Create another
   instance with a different tmux session or definition for isolation checks.
2. Start with System notifications disabled and an empty server preference
   collection. Use a fresh browser context for permission and Service Worker
   checks. Do not print push endpoints, key material, tokens, or provider data.

## Procedure and assertions

1. Open Agent details for a supported SSH/tmux instance. Verify it offers only
   a `Manage in Settings` action for browser notifications. Open the action and
   verify Settings > Notifications contains all three controls, defaulting to
   off. A local, unsupported, or non-tmux instance has no editable target
   preference. The controls are keyed by stable connection definition plus
   tmux session, never by connection-instance ID.
2. Enable `Notify for this connection`. Verify the parent PUT contains only
   the documented preference identity and booleans, the two child controls
   remain off, and no browser permission prompt or notification is triggered by
   changing the server preference.
3. Enable `Agent running to idle` and create a real `running -> relax`
   transition. With the page visible, verify the transition is stored in
   Message Center but no browser notification is shown. Explicitly enable
   System notifications through Settings > Notifications and grant browser permission, then
   hide/unfocus the page and create another transition; verify exactly one
   browser notification is eligible.
4. Enable `Agent running to error` independently using a provider fixture that
   explicitly reports error. Verify `running -> error` can notify while
   `running -> relax` can be disabled, and normal `relax -> running`, tool
   failures, stale snapshots, and disconnects never pass the notification
   filter.
5. From the second shared-target instance, open Agent details and verify it
   reads the same preference. Change one child toggle there and verify the
   first instance reflects the saved value after its next refresh. A different
   tmux session or connection definition remains at default off.
6. Reload, sign out and back in, use the second login session, and restart the
   dedicated backend workload. Verify server preferences persist and are
   available only to the owning user. Local browser opt-in and permission state
   must not silently enable a target preference.
7. Delay or reject one preference PUT. Verify the changed checkbox rolls back
   to its last saved state, the control is re-enabled, and exactly one concise
   error is shown. A subsequent retry saves once and updates the foreground
   notification projection without reloading the application.
8. Disable the parent target switch. Verify both child controls become
   disabled, no transition produces a browser notification, and Message Center
   still records the standard state-change row. Re-enable the parent without
   assuming either child toggle is enabled.
9. Inspect all preference reads and writes. They must not contain endpoint keys,
   provider session or turn IDs, tmux socket fingerprints, prompts,
   transcripts, cwd, model, credentials, or terminal output. Verify a shared
   preference produces one logical eligibility decision even when two live
   connection instances receive the same projection.
10. At phone width verify the controls remain readable, independently
    focusable, and do not overlap the Agent dialog or native input viewport.
    Escape and close return focus to the Agent robot control. Browser
    notification permission, Service Worker registration, and Message Center
    remain separate from the target preference toggles.

## Pass gate and cleanup

Correlate each preference API request with one checkbox action and inspect the
browser diagnostics gate. Fail on default-on state, child-toggle coupling,
instance-specific keying, duplicate notifications, leaked provider data,
unexpected requests, or an unhandled rejection. Capture default, enabled,
rollback, and disabled screenshots.

Restore all preferences to off, remove only the test transitions and
connection instances, close the contexts, unregister the test notification
subscription if one was created, and restore the browser permission/profile
state.
