# Third-Party Notices

Roaminal is a clean-room implementation. The Tabminal repository was used only
as a behavior and interaction reference during early product development; no Tabminal
source or asset is bundled in this repository.

## Behavior reference

- Tabminal `v3.0.40`, commit
  `fbd26d3aff033fd850a6696eccb107520780fd8b`
- License: MIT, as distributed by the reference repository
- Historical use: behavior, protocol, and interaction comparison during early
  product development only

## Direct Go dependencies

The following modules are direct runtime dependencies and retain their upstream
license notices in their module distributions:

- `github.com/creack/pty v1.1.24` - MIT
- `github.com/coder/websocket v1.8.15` - MIT
- `golang.org/x/sys v0.47.0` - BSD-3-Clause

## Direct JavaScript dependencies

The following packages are distributed under MIT licenses unless noted by their
package metadata:

- React, React DOM, TypeScript, Vite, Vitest, ESLint, typescript-eslint,
  `@vitejs/plugin-react`, and `@playwright/test`
- xterm.js packages (`xterm`, `xterm-headless`, `xterm-addon-serialize`, and the
  `@xterm/*` frontend packages)
- `@fontsource/monaspace-neon` and its bundled Monaspace Neon font files -
  SIL Open Font License 1.1; see `LICENSES/OFL-1.1.txt`

The production image includes the terminal worker's production dependency tree
and the notices copied by the `Containerfile`. License text remains owned by
each upstream project; this inventory is not a replacement for those package
notices.
