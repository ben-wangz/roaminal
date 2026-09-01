# Terminal UI Information Hierarchy and Status Reduction

## Purpose

This document supersedes the earlier footer-only design. It defines the
information hierarchy for the authenticated Terminal workspace after removing
duplicated connection identity and connection-state presentations. The
reference image `docs/refactor/main-ui.png` is a visual reference for metric
density and alignment only. Existing product behavior, security boundaries,
responsive behavior, and runtime ownership remain authoritative.

The result must preserve all current capabilities while making each fact have
one clear visual owner:

- the topbar is for Roaminal navigation and optional local resource metrics;
- the active connection identity is carried by the terminal footer;
- Remote monitor is only a remote-host health instrument;
- the footer is the only visible textual Terminal runtime/PTY connection state;
- the Connections panel is an instance inventory and selection surface.

## Scope and non-goals

- Apply this design to the authenticated Terminal workspace.
- Remove the visible selected-connection context/header row from the main
  workspace. Do not replace it with another title row.
- Do not add connection switching, terminal commands, tmux commands, or new
  monitoring requests.
- Do not remove heartbeat or probe mechanisms from the backend. They may remain
  internal data sources, but their connection/probe state must not be rendered
  as competing connection-status text in the Terminal UI.
- Do not use private hostnames, usernames, paths, credentials, URLs, clock
  values, or command output from the reference image as fixtures or product
  data.
- Preserve Terminal, FileSystem, agent, message, notification, fullscreen,
  mobile input, IME, preview, grouping, and accessibility behavior unless this
  document explicitly changes only its presentation location.

## Information ownership

The following ownership rules are mandatory. A value must not be rendered in a
second visible location merely because the second component already has access
to the same object.

| Information | Sole visual owner | Required treatment |
| --- | --- | --- |
| Active connection name | Terminal footer | Show once in the footer identity region. The sidebar may show titles for list selection. |
| Safe user/host/port endpoint | Terminal footer | Show once when available; use the approved safe display projection. |
| PWD | Terminal footer | Show once for the active Terminal. Sidebar keeps it only as non-primary accessible/detail metadata. |
| Terminal runtime/PTY state | Terminal footer | Show the only visible textual `Connected`, `Reconnecting`, `Exited`, or equivalent runtime state. |
| Remote host health | Remote monitor | Show only through the `REMOTE-HEALTH` color state and remote metrics; no connection name or host alias. |
| Roaminal heartbeat state | No Terminal UI element | Keep internal for transport/recovery. Do not show `Connected` or `Reconnecting` for it. |
| Connection instance total | Connections rail icon badge | Show on the `PanelLeft` icon as an app-style numeric badge. Do not repeat the count in the panel header or topbar. |
| Instance inventory identity | Connections cards | Keep title, selection, lifecycle indicator, agent artwork, and actions needed to choose an instance. |
| Terminal type, grid, tmux/ssh context | Terminal footer | Show once in the footer terminal-context region. |
| Local Roaminal resource metrics | Topbar local monitor, if retained by the current shell | Keep metrics scoped to Roaminal. Do not attach the active connection name or heartbeat state to them. |

Different data scopes must be made explicit in labels and component semantics:
topbar resource metrics describe the Roaminal runtime; Remote monitor metrics
describe the connected remote host; footer state describes the active Terminal
runtime or PTY. They must never share an ambiguous connection label.

## Topbar requirements

The topbar remains the global shell header for branding and navigation. It must
not identify or report the active remote/local connection.

Remove from the visible topbar:

- the active connection name currently passed into `SystemStatus`;
- the Roaminal frontend-to-backend heartbeat label and dot;
- `Connected`, `Reconnecting`, or equivalent heartbeat text;
- connection-instance count;
- any selected-connection endpoint or PWD.

The heartbeat connection remains available to internal hooks and recovery logic.
If the existing topbar local monitor continues to show CPU, memory, uptime, or
other Roaminal runtime metrics, those metrics may remain, but they must be
visually and semantically scoped to Roaminal and must contain no active
connection identity or heartbeat-state label. Persistence or backend warnings
may continue through the existing toast/diagnostic path, not as a competing
Terminal connection state.

The left rail remains the sole visible switcher for Connections and Virtual
keyboard. Do not restore a second text-tab or tool-switch control in the
topbar.

## Workspace composition

When the active page is the Terminal workspace, the vertical order is:

1. global topbar;
2. left tool rail and the selected tool surface;
3. Remote monitor band, when the active instance supports it;
4. live Terminal viewport;
5. Terminal footer.

Remove `WorkspaceContextBar` and do not render any replacement active-connection
title above Remote monitor. The main workspace must not contain a separate
connection title, connection type row, endpoint row, or PWD row. The footer is
the only active-connection identity presentation inside the Terminal work
surface.

FileSystem remains a separate workspace mode. Its own file path/tree and
preview controls remain unchanged; this document removes only the generic
selected-connection context row shared with Terminal.

The Terminal viewport owns all remaining height above the footer. Removing the
context row must not cause Remote monitor content to grow into a blank panel or
cause the Terminal to be mounted below another workspace mode.

## Connections rail count badge

The total number of connection instances must be rendered on the existing
Connections rail button, the button containing the Lucide `PanelLeft` icon.

- Render one numeric badge anchored to the top-right of that icon button.
- Use the same compact visual language as the message unread badge: solid
  high-contrast background, short text, and stable absolute positioning inside
  the button.
- Display the current total, including `0` when the list is empty if the
  button remains available.
- The badge must not change the rail button dimensions, shift the icon, or
  affect hit testing.
- The badge must remain visible when the Connections tool is collapsed or
  expanded.
- Expose the count through the button accessible name, for example
  `Connections, 4`; the badge must not be the only accessible representation.
- Add a stable test selector for the badge.
- Remove the count from the Connections panel header and from every topbar
  monitor/status component.

The badge is an inventory count, not an unread indicator. Do not animate it,
increment it as a notification, or reuse message badge state.

## Remote monitor

Remote monitor is solely a remote-host health instrument. It must not repeat the
active connection name, source host alias, SSH type, endpoint, connection
lifecycle, or Terminal runtime state.

### Header

The visible header consists of:

- the literal label `REMOTE-HEALTH`;
- one collapse/expand control on the far right.

Do not render visible status words such as `Available`, `Stale`, `Unavailable`,
`Warming`, `probe unavailable`, or the host name. The `REMOTE-HEALTH` label and
its status accent change color according to the remote health state:

- available: healthy green;
- stale or warming: warning amber;
- unavailable: error red.

The state must still be accessible. Use a semantic status attribute and an
accessible name such as `Remote health: Available`; the accessible name may be
visually hidden, and color must not be the only representation for assistive
technology. No visible text status is permitted in the monitor header.

Remote monitor may be absent for unsupported/non-SSH/non-live instances as it
is today. It must not leave an empty placeholder band when absent.

### Expanded metrics

When expanded, use the metric composition and density of
`docs/refactor/main-ui.png`:

- one compact primary row with CPU, MEM, and DISK resource blocks;
- one compact secondary row with UPTIME, LOAD, AGE, and RTT;
- strong, readable numeric values;
- short uppercase metric labels;
- thin separators aligned to the monitor grid;
- thin usage meters for CPU, MEM, and DISK;
- existing sparklines for CPU, MEM, and RTT where data is available;
- subordinate details for capacity, mount, PID1, load intervals, freshness,
  and probe latency.

The metric labels and values are data, not connection status. `AGE` and `RTT`
remain valid health metrics even though the monitor no longer displays a probe
success/failure sentence. Stale values must retain their existing stale visual
treatment and must not be presented as fresh.

Keep the existing disclosure behavior, retry/degraded semantics, polling
ownership, default collapsed behavior on phone/tablet, and remote monitor
history. Do not create a second monitor poller for the footer or topbar.

### Layout

- The band is full-width within the main panel and directly below the topbar.
- It is compact and must never consume a large blank area.
- The expanded desktop layout uses the reference image's left primary-metric
  group and right secondary-metric group.
- At narrower widths, metrics may reflow to explicit two-column/one-column
  layouts already defined by the responsive system; do not rely on accidental
  overflow or wrapping.
- The collapse control remains keyboard accessible and has a stable test
  selector.
- No monitor field may display a connection name or host alias.

## Terminal footer

The footer is rendered only for the active Terminal workspace and is removed or
hidden with the Terminal mode. It is read-only and cannot execute commands,
switch connections, control tmux, copy paths, or navigate.

### Visible content and order

The footer is one compact horizontal status line with three logical regions.
The complete desktop content, in order, is:

1. Terminal runtime/PTY status indicator and text state;
2. connection name;
3. safe endpoint in `user@host:port` form when available;
4. `PWD` and the compact working directory, such as `~/workspace`;
5. `TERM` and the effective terminal type, such as `xterm-256color`;
6. current grid as `COLS x ROWS`, such as `120 x 32`;
7. terminal context `tmux` when tmux is enabled, otherwise `ssh` for a
   non-tmux SSH connection or `local` for a local connection.

The visual line must follow this conceptual format without duplicating any
field in another footer region:

`[runtime state] [connection-name] [user@host:port] PWD [~/workspace] TERM [xterm-256color] [COLS x ROWS] [tmux|ssh|local]`

The runtime state is the only visible textual connection state in the Terminal
UI. Use `Connected` for the active runtime/PTY, `Reconnecting` while that same
runtime is reconnecting, and the existing clear terminal lifecycle label for
terminated/interrupted states. Do not render a second heartbeat or remote-probe
state in the footer.

### Endpoint and privacy

- Use the existing approved safe endpoint display projection.
- Prefer `user@host:port` for SSH when user, host, and port are available.
- Include the actual configured port, including the default port, so the
  displayed endpoint is unambiguous.
- If only part of the projection is available, render the available safe part
  and do not fabricate missing values.
- Never render passwords, private key material, tokens, raw SSH directives, or
  unapproved private connection data.
- For local connections, omit the SSH endpoint rather than showing a false
  host; keep the connection name, PWD, terminal context, and `local` suffix.
- Long connection names and endpoints ellipsize within their allocated region;
  their complete safe value remains available through a title or accessible
  description.

### Data sources

- Connection name and safe endpoint come from the active connection-instance
  projection. If the current typed projection does not contain the safe
  endpoint fields, add one explicit safe display projection at the existing
  connection/terminal boundary rather than parsing terminal output in the UI.
- PWD comes from the active connection-instance/runtime metadata and follows
  the existing active-pane and metadata update behavior.
- TERM comes from the effective backend-enforced terminal type. The default is
  `xterm-256color` only when that default is actually enforced by the backend.
- COLS and ROWS come from the same authoritative runtime grid that drives PTY
  resize. They must reflect the settled xterm grid after layout changes.
- The tmux/ssh/local suffix comes from the active connection transport and
  tmux-enabled projection. Do not show a tmux version or tmux session name in
  the footer.
- Browser local time is not part of the new footer format and must be removed
  from the visible footer. It must not be used as a refresh trigger.

Missing values use explicit compact unavailable forms such as `N/A` and never
reuse metadata from a previous connection instance. During pending launch,
show the pending connection name when available and `N/A` for runtime-only
fields until the runtime reports them.

### Footer layout and responsive behavior

Desktop:

- Keep one row attached directly below the Terminal viewport.
- Use three stable logical regions: state/identity, PWD, and terminal context.
- The Terminal viewport owns all remaining height.
- Every shrinkable region has `min-width: 0`; no field may overlap another or
  create page-level horizontal scroll.
- Give the terminal context enough fixed priority to keep `TERM`, grid, and
  transport context readable. Let endpoint and PWD ellipsize first.

Tablet and narrow desktop:

- Preserve one row whenever the available width permits it.
- Reduce separators and decorative spacing before hiding secondary endpoint
  detail.
- Preserve runtime state, connection name, PWD, TERM, grid, and transport
  context in that priority order. Use explicit breakpoint rules rather than
  accidental flex wrapping.

Phone:

- Keep the footer compact, safe-area aware, and non-overlapping.
- Preserve runtime state, a shortened connection name, PWD, and `COLS x ROWS`.
- Endpoint text may ellipsize; the full safe endpoint remains accessible.
- Keep the final `tmux`/`ssh`/`local` context when space permits; it may be
  visually compressed at the smallest supported width.
- Hide the footer while the native software keyboard is open if that is still
  required by the existing mobile viewport strategy. Do not place the footer
  over the keyboard or introduce a replacement input field.
- Visual viewport changes must not remount the Terminal or reset scrollback,
  cursor position, input focus, or footer metadata.

## Connection cards

The Connections panel remains an inventory of connection instances. Follow the
existing card interaction and icon semantics, but reduce visible duplication
with the footer.

Each card visibly retains:

- connection title;
- a compact transport/type label such as `SSH` or `Local`, not `SSH
  connection`;
- the lifecycle/attention indicator dot and selected border treatment;
- connection instance ID and SINCE metadata;
- Terminal, FileSystem, drag, action-menu, and agent robot controls;
- terminal preview behavior on pointer-capable desktop environments.

Do not show a visible `Connected`, `Reconnecting`, or remote-probe status
sentence in the card. The card indicator and existing border/attention styles
remain the inventory-level visual status. Its accessible description must still
communicate lifecycle and attention states.

Remove PWD as a primary visible metadata row from the card because active PWD
belongs to the footer. Preserve PWD in the connection-instance data and expose
it through the card's existing safe title/detail/accessibility path so users
can still inspect it without creating a second prominent PWD display. Do not
remove PWD from FileSystem or terminal data flow.

Keep the robot artwork and its state action unchanged. Agent state is not a
connection state and must continue to be visible through the robot artwork,
accessible label, and existing agent dialog/action.

## State transition rules

- On active-instance change, replace footer identity, endpoint, PWD, TERM, grid,
  and transport context atomically. No value from the previous instance may
  remain visible beside the new connection name.
- During runtime reconnect, retain only the last valid metadata for the same
  active instance and show the footer runtime state as `Reconnecting`.
- On runtime termination/interruption, show the terminal lifecycle state in the
  footer and retain final contextual values only for that instance.
- On FileSystem entry, remove the Terminal footer with the Terminal workspace.
- On Remote monitor state changes, update only the `REMOTE-HEALTH` color and
  metric stale treatment. Never add a visible status sentence or connection
  identity.
- On connection list updates, update the rail count badge without moving the
  rail icon or resetting tool selection, collapse state, or active runtime.
- On resize, update footer grid values from the existing runtime resize flow;
  do not add a footer-specific observer or polling loop.

## Accessibility and visual rules

- Visible runtime state text in the footer is required; a colored dot alone is
  insufficient for Terminal connection state.
- Remote monitor status is visually color-coded without visible status words,
  but its semantic accessible name must expose Available, Stale, or Unavailable.
- The connection count badge has an accessible count through the rail button.
- All controls retain keyboard focus rings, labels, titles where useful, and
  existing Escape/backdrop behavior on compact layouts.
- Hidden/inactive Terminal, FileSystem, and tool surfaces must not leave
  focusable controls in the tab order.
- Use existing Solarized-derived tokens: cyan for structure/focus, green for
  healthy state, amber for warning/stale state, and red for errors.
- Do not rely on color alone for any state that is otherwise exposed as text or
  accessibility metadata.
- Keep the base application font at 12 px, use the existing monospace-oriented
  typography, and keep the footer/monitor subordinate to the Terminal content.
- Use stable dimensions, thin separators, small or zero radii, and existing
  Lucide icons. Do not add decorative cards, gradients, or a second icon
  library.
- Respect reduced-motion preferences. The count badge and footer must not
  animate; Remote monitor may retain only its existing reduced-motion-safe
  behavior.

## Implementation ownership

Keep changes within the existing domain boundaries:

- `ShellTopbar` and `SystemStatus`: remove visible heartbeat/active-connection
  status and count; preserve global actions and explicitly scoped local
  metrics if retained.
- `WorkspacePage`: remove the generic workspace context bar and keep the
  Terminal/Remote monitor/footer order stable.
- `RemoteMonitorBand`: implement the `REMOTE-HEALTH` header, color state, and
  reference-style metrics without identity/status text.
- `TerminalFooter`: own the complete active connection identity, safe endpoint,
  runtime state, PWD, TERM, grid, and transport context format.
- `WorkspaceToolRail`: own the connection-instance count badge on the
  `PanelLeft` button.
- `ConnectionSidebar`: remove prominent PWD and connection-state sentences
  while preserving card actions, inventory identity, agent state, preview, and
  grouping behavior.
- Typed backend/frontend terminal and connection projections: add only the
  explicit safe endpoint projection required by the footer. Do not parse
  terminal output or add a parallel store.
- Existing runtime, heartbeat, Remote monitor, FileSystem, message, and
  notification owners retain their data fetching and lifecycle ownership.

## Verification requirements

### Focused frontend tests

Add or update deterministic tests to verify:

- the topbar contains no active connection name, heartbeat state, or connection
  count;
- the Connections rail `PanelLeft` button has one count badge with the current
  total and accessible count, and the panel header does not repeat it;
- `WorkspaceContextBar` is absent from Terminal workspace output;
- Remote monitor renders no connection/host identity and no visible
  Available/Stale/Unavailable/probe text;
- Remote monitor colors and accessible status names map correctly to available,
  stale/warming, and unavailable states;
- Remote monitor expanded metrics render CPU, MEM, DISK, UPTIME, LOAD, AGE, and
  RTT in the reference-inspired order and preserve disclosure behavior;
- the footer renders the exact field order: runtime state, connection name,
  endpoint when available, PWD, TERM, grid, and tmux/ssh/local context;
- the footer contains no clock, remote-probe state, duplicate monitor state, or
  duplicate connection title outside its identity region;
- local, SSH, tmux, pending, reconnecting, terminated, interrupted, missing
  endpoint, and missing grid cases use the defined safe fallbacks;
- active-instance changes replace all footer values atomically with no stale
  cross-instance metadata;
- card PWD is not a prominent visible row but remains available through the
  approved detail/accessibility path;
- no footer-specific polling, resize observer, heartbeat request, or Remote
  monitor request is introduced.

### Browser regression specifications

Update the maintained AI-agent specifications in `tests/playwright/`:

- `workspace/01-terminal-io.md`: verify only the footer exposes the Terminal
  runtime state and that active-instance identity/PWD metadata is consistent.
- `workspace/02-sidebar-selection-and-preview.md`: verify the rail count badge,
  card selection, card actions, and atomic footer replacement.
- `workspace/08-resize.md`: verify footer `COLS x ROWS` follows actual xterm
  resize at desktop, tablet, phone, monitor, search, and keyboard states.
- `workspace/09-system-status.md`: verify no visible heartbeat/active-connection
  status is rendered in the topbar while retained local metrics remain scoped.
- `workspace/10-remote-monitor.md`: verify `REMOTE-HEALTH` color states,
  reference-style metrics, no host/name/status text, disclosure behavior, and
  no duplicate polling.
- `workspace/11-responsive-and-accessibility.md`: verify no overlap, wrapping
  failure, horizontal overflow, keyboard obstruction, or hidden focusable
  controls.
- `workspace/19-main-ui-upgrade.md`: update the shell hierarchy and remove the
  obsolete selected-context row and duplicate count/status requirements.

The browser regression run must continue to use the existing diagnostics gate.
Fail on React lifecycle errors, duplicate terminal output, stale cross-instance
metadata, duplicate requests, status text rendered in the wrong owner, raw
endpoint secrets, visual overflow, or inaccessible hidden controls.

## Completion criteria

This design is implemented only when all of the following are true:

- no Terminal UI element visibly renders the Roaminal heartbeat state;
- no Remote monitor header visibly renders probe success/failure words;
- no generic workspace active-connection title row remains;
- the Connections count appears only as the `PanelLeft` rail badge;
- Remote monitor visibly consists of `REMOTE-HEALTH` with color state and the
  CPU/MEM/DISK plus UPTIME/LOAD/AGE/RTT instrument layout;
- the footer is the only visible textual Terminal runtime/PTY connection
  state and follows the specified connection/PWD/TERM/grid/transport order;
- connection cards remain useful for choosing and operating instances without
  a second prominent PWD or connection-state presentation;
- Terminal, FileSystem, agent, message, notification, grouping, preview, tmux,
  IME, mobile, fullscreen, and accessibility behavior remains functional;
- focused tests, typecheck, lint, build, backend tests, and the complete
  deployed browser regression set pass;
- the final review maps every ownership, fallback, responsive, and state rule
  in this document to an implementation owner and test.
