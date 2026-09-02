# PW-SETTINGS-001: Unified configuration center

Priority: P0. Capabilities: core. Viewports: desktop, tablet landscape,
tablet portrait, phone portrait, and phone landscape.

## Preconditions

1. Complete the Helm deployment and browser diagnostics gates in
   [`tests/playwright/README.md`](../README.md). Authenticate through the
   visible login UI and record the initial `bootId`.
2. Keep the existing connection definitions, SSH keys, appearance record, and
   notification preferences available for cleanup. Do not use a fixture value
   as a hard-coded connection name.

## Cases

1. With a live connection instance selected, click the far-left `Settings`
   rail button. Verify the button is the only Settings entry point, the
   topbar has no Settings button, the rail remains visible, and the terminal,
   monitors, footer, workspace tool surface, and FileSystem preview are
   replaced by the Settings page. The settings button is the compact return
   control; no large Back button is present.
2. Click Settings again and verify the unchanged workspace returns with the
   previous tool selection and collapsed/expanded state. On phone-sized
   viewports verify focus returns to the Settings rail button.
3. Verify the desktop page has the application rail, a separate left settings
   navigation column, and one right content column. The definitions section
   shows the `CONNECTION DEFINITIONS` header, three aligned source cards, one
   filter/action row, and one five-column definition surface. No content is
   rendered below another selected section. Verify the settings page has no
   horizontal overflow.
4. At tablet and phone sizes verify the same DOM content becomes one column,
   the rail becomes a 44px horizontal strip in the order Connections, Virtual
   keyboard, Files, Settings, Help, and the four section buttons wrap without
   horizontal overflow. Each section button remains a readable touch target.
5. Exercise each settings section in order: Connection definitions, SSH keys,
   Interface, and Notifications. Verify only the selected section is visible,
   the selected item has `aria-current="page"`, the section heading is
   focusable after navigation, and returning to a section restores its scroll
   position.
6. In Connection definitions filter by alias, hostname, and user using mixed
   case and surrounding whitespace. Verify filtering is local. Verify Local,
   source status, managed-key count, trust, Tmux, warning/read-only facts, and
   Start/Edit/Duplicate/Delete actions retain their previous behavior. Add or
   edit a definition through `Connection name`; duplicate names remain in the
   modal with an accessible error and no partial save. Verify the one Refresh
   action loads definitions and SSH keys once each and keeps last-known data
   when one read fails.
7. In SSH keys verify the inventory, public-key copy, generation, read-only
   safeguards, reference warning, and delete behavior. Capture only the
   public key during the explicit copy action; never put private bytes in
   browser state, logs, diagnostics, or screenshots.
8. In Interface verify the existing appearance controls default to 12px,
   accept only integer values from 10 through 32, keep the live preview stable,
   and preserve explicit Save/Reset behavior. Select every FileSystem refresh
   option and verify it is written immediately and is read by the FileSystem
   title-bar control. Change appearance while a workspace connection is live
   and verify the same runtime and WebSocket remain. Leaving with an unsaved
   appearance draft asks whether to discard it.
9. In Notifications verify the global browser-delivery state and explicit
   user-gesture switch. Verify the page does not request permission on entry.
   For every current Tmux-enabled SSH definition verify the parent target
   switch and independent `Agent running to relax` and `Agent running to error`
   switches. Correlate each PUT with one toggle, verify optimistic rollback on
   failure, and verify target identity uses only the stable definition ID and
   Tmux session name. The Agent dialog's `Manage in Settings` action opens and
   focuses the matching target without a second preference read.
10. Trigger heartbeat cycles, terminal redraws, section changes, message
    updates, and unrelated dialogs while Settings is open. Verify there are no
    duplicate definition/key/preference requests, duplicate preview xterms,
    WebSockets, console warnings/errors, uncaught exceptions, failed requests,
    leaked credentials/private key material, or `net::ERR_INSUFFICIENT_RESOURCES`.

## Cleanup and pass gate

Restore the appearance and FileSystem browser-storage values, notification
preferences, definitions, keys, and connection instances. Inspect every
diagnostic event before reporting `PASS`; a visual screenshot alone is not a
pass.
