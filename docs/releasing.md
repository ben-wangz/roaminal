# Release Automation

This document is for repository maintainers and release agents. Deployment
users install published artifacts as described in [deployment](deployment.md).

Roaminal has one ForgeKit app, the `roaminal` Chart, with a linked
`roaminal-runtime` container. Chart and runtime versions are independent;
Chart `appVersion` and `image.tag` match the linked runtime version.

## Trigger

After a release-only commit passes the remote `lint` workflow, an annotated
`roaminal-v<chart-semver>` tag triggers publication:

| Workflow | Result |
| --- | --- |
| `release-container` | `ghcr.io/ben-wangz/roaminal:<runtime-version>` when the runtime changed |
| `release-chart` | `oci://ghcr.io/ben-wangz/roaminal-charts/roaminal:<chart-version>` |

The Chart workflow waits for the exact runtime image. Chart-only releases reuse
the existing runtime image. No maintainer manually pushes images or Charts.

## Maintainer workflow

Use [`.agents/skills/release-version/SKILL.md`](../.agents/skills/release-version/SKILL.md)
for the complete command sequence. It requires a clean `main` checkout,
ForgeKit-only version mutations, a remote lint gate, an annotated tag, and
release-workflow monitoring. Focused local checks are optional; the remote
`lint` workflow is the required quality gate.

Use ForgeKit to inspect the version:

```sh
FORGEKIT_BIN="$(bash ./setup/forgekit.sh)"
"$FORGEKIT_BIN" --project-root "$PWD" version get roaminal --output json
```

Do not edit `container/VERSION`, Chart `version`, `appVersion`, or `image.tag`
by hand. Do not release documentation, tests, or tooling without a changed
published artifact. Never move or recreate a pushed release tag.
