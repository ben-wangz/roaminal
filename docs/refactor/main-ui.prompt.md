# Roaminal Main UI Visual Design Prompt

Use `docs/refactor/screenshot.png` as the primary visual reference. Generate one polished, high-end desktop web application UI concept image for Roaminal, a browser-based remote connection workspace. The goal is to explore a premium CRD-inspired command-room dashboard direction that can be implemented in the existing product. This is a visual design reference, not a marketing page and not a code mockup.

## Product context

Roaminal lets a user manage several live remote connection instances and work inside a terminal attached to a tmux session. The main workspace currently has these functional areas:

- A left workspace tool area containing connection instances grouped under manually managed groups such as `Ungrouped`. Each connection instance shows a title, connection type, status indicator, connection ID, working directory, creation time, terminal preview, and action controls.
- A shared top bar showing the current connection, connection count, remote/system health, CPU, memory, uptime, load, disk, probe latency, messages, settings, fullscreen capability, auth sessions, and sign out.
- A remote monitor for the selected SSH connection, with resource metrics and compact historical sparklines. It can be expanded or collapsed.
- A large terminal work area for the selected connection. The terminal is the primary work surface and must remain visually dominant.
- A compact virtual keyboard tool that can replace the connection tool area on desktop. The tools are mutually exclusive when expanded, and either can be collapsed.
- A FileSystem workspace exists but is not the focus of this image. Do not introduce a new filesystem panel into this concept.
- A compact message center is opened from the bell icon and reports agent state changes. Do not turn it into a large dashboard or chat panel.

The supplied screenshot is the current main screen. Use it to understand the existing density, hierarchy, terminal-first workflow, connection cards, robot state artwork, remote monitor, and Solarized-inspired dark palette. Improve the information hierarchy and visual coherence without inventing new product capabilities.

## Design direction

Create a refined, production-quality command-room interface with a restrained CRD-inspired visual language:

- Dense, calm, technical, and highly scannable; designed for repeated operational use rather than presentation or marketing.
- Keep the terminal as the largest uninterrupted area and the central visual anchor.
- Establish a clear three-level hierarchy: global shell, selected connection context, and active work surface.
- Make the left-side tool switch between Connections and Virtual keyboard feel like one coherent control, not two unrelated panels or browser tabs. Use a compact icon-based segmented control or tool rail with a strong active state; do not use large text tabs.
- Keep the connection list usable with many instances. Show group headers, counts, collapse affordances, search, and compact but expressive connection cards. Preserve room for the robot state artwork at the right side of each card, where it acts as a status signal rather than decoration.
- Give the selected connection a precise, unmistakable state using a restrained accent line, surface change, or focus frame. Do not rely on color alone.
- Make the remote monitor look like an intentional instrument panel: align metrics, use consistent measurement blocks, keep labels compact, and prevent the monitor from competing with the terminal. Its collapse affordance must be obvious but quiet.
- Keep the message bell, settings, fullscreen, sessions, and sign-out controls discoverable in the top-right without making the header visually noisy.
- Use small, familiar icons with clear hit areas. Prefer iconography over redundant labels where the action is familiar, but retain short labels for critical navigation when needed.
- Keep cards and panels square or only subtly rounded, with crisp separators and a strong grid. Do not nest decorative cards inside cards.
- Use the existing dark Solarized-like foundation as a starting point, but improve contrast, surface separation, and semantic status colors. The result must not read as a single flat blue or teal block. Use carefully balanced cyan, amber, green, red, warm light text, and near-black surfaces.
- Pixel-art robot status illustrations may be used in the connection cards. They should remain clear at the intended card size and should communicate state through posture and silhouette. Do not replace them with generic glossy 3D mascots.
- Typography should be compact, monospaced or technical for terminal-adjacent data, with a restrained sans-serif or humanist companion for navigation if useful. Keep text legible and avoid oversized headings.

## Layout requirements

Render a complete 16:9 desktop viewport, approximately 1440 x 900 or 1600 x 1000, showing the actual application in use:

1. A narrow global header spans the top of the application. It includes the Roaminal mark, the active connection context, compact system health metrics, and right-side utility actions.
2. A left tool column is approximately 280-320 px wide. Its header contains the unified Connections / Virtual keyboard switcher and a collapse control. The screenshot should show Connections selected.
3. The left column contains a search field, an `Ungrouped` group with a count, and three or four connection instance cards. Include at least one selected card, one active/live card, one card with a non-ready agent state, and one card with a different host. Keep cards compact enough that multiple instances remain visible.
4. The selected connection card includes a small terminal preview or activity preview behind the card content, but the title, status, ID, PWD, and SINCE metadata remain readable. Put a medium-large pixel-art robot state illustration on the right side without covering the metadata or action controls.
5. The main panel starts with a compact remote identity/health band. Show a selected remote host, availability status, aligned CPU/MEM/DISK meters, uptime/load, age, RTT, and one small sparkline. The band may be expanded in the concept, but it must remain much shorter than the terminal.
6. Below the monitor, show a large live terminal surface occupying most of the viewport. It should contain believable but intentionally generic terminal output, short shell prompts, and a visible cursor. Do not reproduce private text, repository names, credentials, URLs, or long conversation transcripts from the reference screenshot.
7. Include a minimal terminal footer/status line with working directory and terminal dimensions or connection state.
8. The visual result must clearly read as one cohesive application shell, not as a collection of floating dashboard widgets.

## Interaction cues to represent visually

The generated image should make these behaviors apparent through visual affordances, even though it is static:

- Clicking a connection instance selects it and changes the active terminal context.
- The terminal tool is represented by the terminal icon and the FileSystem tool is represented by the folder icon in the connection card actions; do not add a new large workspace tab bar.
- The left tool column and virtual keyboard are alternate tools. A compact unified switcher controls which one is visible; they must not appear as two simultaneously expanded sidebars.
- Group headers can collapse and show the number of contained connection instances.
- The remote monitor can collapse to preserve terminal space.
- The message bell has a small unread badge and opens a lightweight message popover, not a full page.
- A fullscreen icon may be visible in the global header, but do not render the browser or application as if it were already in a special fullscreen mode.

## Responsive intent

Although the output is a desktop concept, design every major region so it can translate cleanly to a narrow tablet or phone viewport:

- Preserve the terminal as the primary surface.
- Let the connection tool and virtual keyboard become an overlay or bottom tool area on narrow screens instead of forcing a desktop-width sidebar.
- Keep controls reachable with touch-sized hit areas and avoid hover-only affordances.
- Avoid fixed-height regions that would be hidden by a mobile browser keyboard or safe-area insets.
- Do not show a large persistent mobile input box or a redundant mobile shortcut bar.

## Strict exclusions

- No marketing hero, pricing, onboarding, landing-page copy, or promotional illustration.
- No large explanatory text blocks, charts unrelated to remote health, kanban boards, file browser, chat application, or command execution controls outside the terminal.
- No `Open in terminal` action, script runner, command palette, or extra automation panel.
- No browser chrome, device frame, laptop mockup, or perspective presentation. Show only the application viewport.
- No gradients used as the dominant visual treatment, glowing blobs, decorative orbs, glassmorphism, excessive shadows, oversized rounded cards, or purple-heavy palette.
- Do not copy the exact terminal transcript or private content visible in the reference image.
- Do not introduce unreadable pseudo-text. Use short, legible UI labels such as `Connections`, `Ungrouped`, `CPU`, `MEM`, `DISK`, `RTT`, and `Connected` only where text is needed.

## Output

Produce a single high-resolution UI concept image with sharp alignment, crisp typography, realistic spacing, and implementation-aware proportions. Prioritize the complete shell and layout relationships over decorative detail. The image should be suitable for an engineer to use as a visual target when refactoring the existing Roaminal main screen.
