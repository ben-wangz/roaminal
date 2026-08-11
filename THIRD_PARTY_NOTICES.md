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
- `gopkg.in/yaml.v3 v3.0.1` - MIT

## Direct JavaScript dependencies

Runtime packages:

- React and React DOM - MIT
- `@xterm/xterm`, `@xterm/headless`, `@xterm/addon-fit`,
  `@xterm/addon-ligatures`, `@xterm/addon-progress`, `@xterm/addon-search`, and
  `@xterm/addon-serialize` - MIT
- `lucide-react` - ISC
- `@fontsource/monaspace-neon` and its bundled Monaspace Neon font files -
  SIL Open Font License 1.1; see `LICENSES/OFL-1.1.txt`

Development-only packages include TypeScript, Vite, Vitest, ESLint,
typescript-eslint, and `@vitejs/plugin-react`.

The production image includes the terminal worker's production dependency tree
and the notices copied by the `Containerfile`. License text remains owned by
each upstream project; this inventory is not a replacement for those package
notices.

## Base image system tools

The production image also installs the distribution `openssh-client` package.
OpenSSH is invoked as the system SSH runtime; Roaminal does not embed or
redistribute an SSH implementation.
