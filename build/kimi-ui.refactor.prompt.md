# Kimi Code Task: Assess and Plan a Roaminal UI/Interaction Refactor

You are Kimi Code working in the Roaminal repository. Your task in this run is
an evidence-based assessment of the existing frontend UI and interaction model,
followed by a complete refactor plan. Do not implement the refactor in this
run. Do not modify application source code, tests, Helm values, or runtime
configuration. The only required repository write is one planning document in
`docs/todo/`.

## 1. Repository rules

Read `AGENTS.md` before doing anything else and follow it. In particular:

- Use `connection definition` and `connection instance` for product concepts.
- Frontend code is in `frontend/`; backend code is in `backend/`.
- `chart/` is the only Kubernetes template source.
- Existing browser regression specifications are maintained in
  `tests/playwright/`; do not create a second checked-in Playwright suite.
- Documentation is concise and English-only.
- Do not edit managed release versions during this assessment.

Read `tests/playwright/README.md` completely. Treat it as the browser test
contract, especially the Helm deployment gate, direct Service requirement,
secure-context requirement, standard viewports, and browser diagnostics gate.

Inspect the current frontend source, styles, state management, API adapters,
existing tests, and the relevant backend contracts before forming conclusions.
Do not assume that an old screenshot or an old planning document represents the
current behavior.

## 2. Product context and decisions that are already fixed

The assessment must preserve these product decisions. Do not reopen them as
generic design questions unless the current implementation makes one technically
impossible; in that case record concrete evidence and a narrowly scoped
alternative.

- Roaminal is a work-oriented connection platform, not a marketing site.
- The left sidebar contains connection instances, not connection definitions.
- Connection instances may be manually placed into user-created groups.
- A group containing connection instances cannot be deleted.
- A user-created group has a maximum of 10 connection instances.
- `Ungrouped` has no capacity limit.
- A newly created connection instance enters `Ungrouped`.
- A collapsed group shows its connection count and does not render all cards.
- Connection definition grouping is out of scope for this assessment.
- Clicking the Agent control on a connection instance opens Terminal.
- Clicking the folder/FileSystem control opens FileSystem.
- Terminal and FileSystem are parallel workspace modes. Do not add a Terminal /
  FileSystem tab strip.
- In Terminal, the terminal surface is the primary workspace and must not be
  pushed below a large empty monitor or control area.
- FileSystem is a tree plus preview workspace. Single click selects; double
  click opens a file preview.
- A connection instance that shares a tmux session uses the active-pane
  semantics already defined by the product.
- FileSystem has no script execution, arbitrary command execution, or
  “Open in terminal” action.
- Upload file and upload folder are one unified upload action, and upload must
  require a confirmation dialog before the network request starts.
- Do not introduce an application-level “full screen” action.
- Directory and file context menus must work with mouse right-click, keyboard
  context-menu invocation, and long press on touch devices.
- The FileSystem toolbar has one global refresh. A directory context menu has a
  directory-only Refresh. Re-expanding a directory follows the existing lazy
  refresh policy. Auto-refresh is user-configurable.
- The preview must not reset the user's scroll position, page, zoom, or other
  browsing state merely because the tree or root refreshes.
- The virtual keyboard is independent from the connection list, can collapse
  and expand, and is mutually exclusive with an expanded connection list.
- The virtual keyboard common key set includes `ESC`, `Tab`, `^C`, `|`, `~`,
  `/`, `Enter`, and the four arrow characters. Do not replace arrow characters
  with the words Up/Down/Left/Right.
- The mobile native/common shortcut bar is not a second virtual keyboard.
- On mobile, the command composer must remain visible above the software input
  method and provide a focused editing/sending experience.
- The default application font size is 12px unless the current code or an
  explicit accessibility requirement proves that a local exception is needed.
- The UI must remain usable on desktop, tablet, and phone portrait/landscape.

## 3. Environment and evidence collection

First determine whether the repository and test environment are usable. Do not
claim a UI issue based only on static code inspection when it can be observed in
the browser.

Use the Helm-managed test release. For this repository's shared `develop`
environment, probe and use these defaults when no operator-specific values are
provided:

- Helm release: `roaminal`
- Namespace: `develop`
- Direct Service URL: `http://roaminal.develop.svc.cluster.local:9846`
- HTTPS Ingress URL: `https://roaminal-e2e.pve.lab.geekcity.tech:22443`

Before creating a browser context, perform the deployment gate from
`tests/playwright/README.md`. Resolve the actual release and namespace from
environment variables if they are present; do not silently use a different
deployment. If the direct Service is reachable, use it for ordinary browser
cases. Use the HTTPS URL for TLS, secure-origin, and `wss:` checks. Do not use a
port-forward or a locally started frontend/backend as a substitute.

Never print or place a password in a command argument, screenshot, report, or
the planning document. Read it from the configured Secret into an environment
variable only when the local test environment permits that operation.

Use Playwright or the repository's existing browser tooling to collect evidence
at all five standard viewports:

| Name | Viewport |
| --- | --- |
| desktop | 1440 x 900 |
| tablet landscape | 1024 x 768 |
| tablet portrait | 768 x 1024 |
| phone portrait | 390 x 844 |
| phone landscape | 844 x 390 |

For each viewport, capture screenshots of the actual working UI where the state
is meaningful. Prefer screenshots that show the sidebar, terminal or FileSystem
workspace, monitor areas, and virtual keyboard in context. Also collect DOM
measurements for viewport overflow, fixed/sticky regions, overlapping boxes,
unexpected scroll containers, text clipping, and controls that move when state
changes. A screenshot is evidence, not the sole assertion.

Install diagnostics before the first `page.goto()` and retain them through
cleanup:

- console messages, including warnings and errors, with source location;
- uncaught page errors;
- failed requests;
- unexpected 4xx/5xx responses and the response body for expected negative
  cases;
- WebSocket URL, close code, close reason, socket errors, and relevant frame
  ordering.

Use the existing regression specifications as behavioral references. Exercise at
least these workflows when their fixtures are available:

1. Login, initial connection-instance loading, sidebar open/close, group
   creation, collapse/expand, drag or keyboard reorder, move-to-group, group
   capacity, and group deletion rules.
2. Switch between Terminal and FileSystem through the connection controls;
   verify that one workspace replaces the other instead of being appended below
   it.
3. Terminal with local and SSH/tmux connection instances, including monitor
   states, active pane indicators, virtual keyboard expansion/collapse, and
   narrow screens.
4. FileSystem root loading, lazy tree expansion, selection, double-click
   preview, Markdown/source and rendered scrolling, image/PDF/video or other
   supported previews, targeted refresh, global refresh, auto-refresh, and
   preview state preservation.
5. Mouse context menu, keyboard context-menu invocation, touch short press,
   touch movement cancellation, and long press. Verify menu placement remains
   inside the viewport.
6. Unified upload action, confirmation dialog, cancellation before submission,
   upload progress/failure/success, and the final tree refresh.
7. Mobile command input while the native software keyboard is open. Verify that
   the text being edited and the send control are visible, that Enter is not
   accidentally submitted as ordinary text, and that the layout recovers after
   the keyboard closes.
8. Connection definition editor and source-error states. Verify that the last
   valid tmux/FileSystem values are retained and that unavailable source data
   does not create synthetic defaults or silently disable an existing option.

When a fixture is unavailable, record an explicit blocked or skipped evidence
item and continue with all safe observations. Do not turn a missing Helm
deployment into a successful “not applicable” result.

## 4. Assessment scope

Evaluate the current UI and interaction at four levels.

### Information architecture

- Is the distinction between connection definitions, connection instances,
  groups, workspace modes, monitors, and virtual keyboard immediately legible?
- Does the sidebar support scanning and repeated operations when there are many
  instances and groups?
- Are the currently active connection instance and active workspace mode always
  obvious?
- Does the page hierarchy make the terminal or FileSystem work surface dominant?
- Are status, error, loading, empty, unavailable, and retry states located where
  the user needs them?

### Interaction design

- Are click, double click, right click, keyboard, drag/drop, and long press
  behaviors predictable and non-conflicting?
- Do controls have a clear target size and do they avoid accidental activation
  on touch devices?
- Are destructive and network-affecting actions confirmed at the right point?
- Are focus, Escape, Enter, Tab, arrow navigation, context-menu keys, and focus
  return behavior coherent?
- Do asynchronous updates preserve selection, scroll, preview state, cursor,
  expanded groups, and active connection state?
- Are retry actions honest about whether a failure is retryable or requires a
  fresh connection instance?
- Are transient refreshes debounced or otherwise prevented from disrupting the
  user's current task?

### Visual system and layout

- Assess hierarchy, density, spacing, typography, contrast, borders, active
  indicators, icons, control affordances, and text wrapping.
- Identify panels that consume disproportionate height or width, blank regions,
  nested cards, unnecessary borders, and competing visual focal points.
- Check fixed dimensions and responsive constraints for tree rows, preview
  surfaces, monitors, keyboard keys, buttons, dialogs, and menus.
- Check that no text, icon, menu, tooltip, dialog, or status badge overlaps or
  escapes its parent at any target viewport.
- Check mobile safe areas, software keyboard resizing, portrait/landscape
  transitions, and touch hit areas.
- Reuse the existing design language and icon library where it is sound. Do not
  propose decorative marketing elements, oversized hero content, gradient
  blobs, or visual changes unrelated to the workbench workflow.

### Technical maintainability relevant to UI

- Identify duplicated layout/state logic, components with too many unrelated
  responsibilities, unstable keys, effects that cause refresh loops, and API
  state that is inferred from presentation rather than modeled explicitly.
- Identify places where a visual bug is caused by lifecycle, sizing, transport,
  caching, or event propagation rather than CSS alone.
- Identify state that should be shared, local, persisted, or derived, and explain
  why. Do not propose a new state-management library without evidence.
- Check whether proposed UI changes would require an API/contract change or can
  remain frontend-only.

## 5. Required analysis discipline

For every finding, distinguish facts from hypotheses.

- A fact must include evidence: viewport, workflow, screenshot name or DOM
  selector, relevant source file, or observed network/console behavior.
- A hypothesis must state what remains unverified and how the implementation
  phase should verify it.
- Prioritize findings by user impact and regression risk: P0 blocks a primary
  workflow, P1 materially harms repeated work or responsive use, P2 is a
  consistency or maintainability improvement.
- Do not list subjective preferences as defects without tying them to user
  workflow, accessibility, hierarchy, performance, or maintainability.
- Do not recommend changing a stable behavior merely because another product
  uses a different pattern. External design research is optional; if used, cite
  the source and translate it into a Roaminal-specific decision.
- Resolve ordinary implementation choices in the plan. Leave an open question
  only when it genuinely requires product-owner input or external information.

## 6. Required deliverable

Write exactly one English planning document:

`docs/todo/ui-refactor-plan-for-kimi.md`

The document must be self-contained. Another AI agent must be able to implement
the approved UI/interaction refactor from it without reopening basic product
decisions. It must not require this prompt, a screenshot directory, or another
`docs/todo` file to understand the target behavior. Include enough references
to the codebase for implementation, but do not copy large source files into the
plan.

Use this structure:

1. **Assessment scope and evidence baseline**
   - repository revision, Helm release/namespace, runtime version, boot ID,
     tested URL type, date, available fixtures, and all tested viewports;
   - list of screenshots and diagnostic results;
   - explicit limitations or skipped workflows.
2. **Current product model**
   - concise description of sidebar, groups, connection instance lifecycle,
     Terminal, FileSystem, monitors, virtual keyboard, dialogs, and mobile
     input behavior as actually implemented.
3. **Prioritized findings**
   - each finding has ID, priority, affected viewport/workflow, observed
     behavior, desired behavior, evidence, likely cause, and regression risk.
4. **Target information architecture and layout**
   - page regions, ownership of primary/secondary surfaces, workspace mode
     transitions, sidebar/group layout, monitor placement, FileSystem tree /
     preview proportions, virtual keyboard placement, and responsive behavior;
   - define dimensions or constraints where a later agent would otherwise have
     to guess.
5. **Target interaction specification**
   - exact behavior for click/double click, drag/drop, group actions, context
     menus, long press, keyboard navigation, dialogs, upload confirmation,
     refresh, auto-refresh, preview persistence, monitor errors, transport
     errors, and mobile software-keyboard states;
   - include loading, empty, disabled, unavailable, retryable, non-retryable,
     success, cancellation, and cleanup states.
6. **Component and state boundaries**
   - proposed frontend component ownership, state ownership, event/data flow,
     persistence boundaries, and API/contract implications;
   - identify which existing components should be split, merged, retained, or
     deleted, with reasons rather than implementation code.
7. **Visual system adjustments**
   - typography, spacing, colors, borders, icon usage, control sizes, focus
     styles, contrast, responsive constraints, and design tokens;
   - preserve the product's workbench character and avoid decorative scope
     creep.
8. **Implementation sequencing**
   - ordered phases with dependencies, migration/compatibility notes, and
     checkpoints. Each phase must leave the application buildable and testable.
9. **Acceptance matrix**
   - map each change to existing `tests/playwright/` specifications or to a
     clearly named extension of one of them;
   - include desktop, both tablet orientations, both phone orientations,
     keyboard/touch cases, diagnostics assertions, and visual/layout checks;
   - do not create a second Playwright suite.
10. **Risks and genuinely blocking decisions**
    - list only decisions that cannot be resolved from repository evidence or
      the fixed product decisions above.
11. **Definition of done**
    - objective implementation, accessibility, responsive, diagnostics, and
      regression criteria.

The plan should be detailed enough to guide implementation, but it must remain
a design and refactor specification rather than a patch. Prefer tables and
short precise prose over pasted code. Use repository-relative file paths and
stable selectors/component names where useful.

## 7. Stop conditions and final response

Stop after writing and reviewing the planning document. Do not modify
`frontend/`, `backend/`, `tests/`, `chart/`, or release metadata. Do not commit
the planning document unless explicitly asked.

Before finishing:

- verify the document is English-only and internally consistent;
- verify every P0/P1 finding has an evidence reference and an acceptance check;
- verify the target design does not contradict the fixed product decisions;
- run `git diff --check` and report its result;
- report the created document path, environment/fixture limitations, and the
  evidence commands or browser cases that were actually completed.

Your final response must be concise and must clearly say whether the assessment
was completed, partially completed, or blocked. Do not claim that the UI was
refactored; this run produces the plan only.
