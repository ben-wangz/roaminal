---
name: release-version
description: |
  Release Roaminal with ForgeKit by bumping the linked runtime and Helm Chart,
  validating the release commit, pushing a roaminal-v semantic-version tag, and
  monitoring GitHub Actions. Use for runtime or Chart version bumps, release
  commits, release tags, artifact verification, or release failure diagnosis.
---

# Release Roaminal

This skill targets ForgeKit `v0.6.1`. Use it only for an explicitly requested
Roaminal release. It may commit, push, create a tag, and monitor workflows only
after the user has asked for that release operation.

## Repository Contract

Roaminal has one top-level ForgeKit app:

- app: `roaminal` (chart/)
- linked container: `roaminal-runtime` (container/)
- release tag: `roaminal-v<chart-semver>`
- image: `ghcr.io/<owner>/roaminal:<runtime-semver>`
- chart: `oci://ghcr.io/<owner>/roaminal-charts/roaminal:<chart-semver>`

Read linked runtime metadata from the `linked` array returned by:

```bash
FORGEKIT_BIN="$(bash ./setup/forgekit.sh)"
"$FORGEKIT_BIN" --project-root "$PWD" --output json version get roaminal
```

Do not call `version get roaminal-runtime`: ForgeKit `v0.6.1` only resolves
top-level registry apps with `version get`, while linked targets are valid
arguments for `version bump` and `publish`.

ForgeKit owns `container/VERSION`, Chart `version`, Chart `appVersion`, and the
default `image.tag`. Never edit any of these fields manually. The private
frontend and terminal-worker package versions remain `0.0.0`.

## Preflight

Run these checks before deciding or changing a version:

```bash
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
cd "$PROJECT_ROOT"
test "$(git branch --show-current)" = main
test -z "$(git status --short)"
gh auth status
FORGEKIT_BIN="$(bash ./setup/forgekit.sh)"
"$FORGEKIT_BIN" --version
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal
```

Preserve unrelated user changes. Stop and report the conflict if a required
release file is already modified. Do not use `git reset`, force push, or move an
existing release tag.

## Choose The Bump

Inspect changes since the previous `roaminal-v*` tag and apply exactly one
increment. Use `patch` by default; require an explicit user decision for
`minor` or `major` when the change is not clearly classified.

Runtime change:

```bash
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump container roaminal-runtime patch
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump chart roaminal patch --sync
```

Chart-only change:

```bash
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump chart roaminal patch --sync
```

Do not create a release for documentation, test, or tooling changes that do
not alter a published artifact. A runtime release must also bump the Chart;
the Chart release tag always uses the Chart version, not the runtime version.

## Review And Validate

Review only the intended version files first:

```bash
git diff -- container/VERSION chart/Chart.yaml chart/values.yaml
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal --output json
```

Confirm that linked runtime version, Chart `appVersion`, and `image.tag` are
equal, and that untouched artifacts did not change. Run focused local checks
only when they help diagnose the changed module. Do not require a local full
ForgeKit lint, container build, or Chart package; the remote `lint` workflow is
the required release gate.

## Commit And Tag

Create one release-only commit after review:

```bash
git add container/VERSION chart/Chart.yaml chart/values.yaml
git commit -m "chore(release): bump Roaminal release versions"
git push origin main
```

Wait for the `lint` workflow whose commit SHA exactly matches the pushed commit:

```bash
gh run list --workflow lint.yaml --commit "$(git rev-parse HEAD)" \
  --json databaseId,status,conclusion,url
gh run watch <lint-run-id> --interval 10 --exit-status
```

Only after remote lint succeeds, verify the worktree is clean, `HEAD` is on
`main`, `origin/main` has not advanced, and the tag does not already exist.
Create and push an annotated tag using the Chart version from ForgeKit:

```bash
VERSION_JSON="$($FORGEKIT_BIN --project-root "$PROJECT_ROOT" --output json version get roaminal)"
VERSION="$(VERSION_JSON="$VERSION_JSON" node -p 'JSON.parse(process.env.VERSION_JSON).data.app.value')"
git tag -a "roaminal-v${VERSION}" -m "Roaminal ${VERSION}"
git push origin "roaminal-v${VERSION}"
```

Tags must be stable SemVer or valid prerelease SemVer without `+build` metadata.
Never delete and recreate a pushed release tag.

## Monitor The Release

The tag triggers `release-container` and `release-chart` independently. The
container job skips when the linked runtime version is unchanged since the
previous release. The Chart job waits for the exact GHCR image before publishing
the Chart, so a missing image fails the release rather than producing an
uninstallable Chart.

Monitor both workflows and report their URLs and terminal conclusions:

```bash
gh run list --limit 20 --json databaseId,workflowName,headBranch,status,conclusion,url \
  --jq '.[] | select(.headBranch == "roaminal-v'${VERSION}"'")'
```

For each pending run, use `gh run watch <run-id> --interval 10 --exit-status`.
Report the tag, commit, Chart version, runtime version, image reference, Chart
OCI reference, run IDs, and any skipped container job. A workflow failure is
not a release success; include the failed job and URL. Retry a transient
workflow without moving the tag. Fix code with a new version and new tag.

ForgeKit `--multi-tag` publishes only the full version for current `0.x` or
prerelease versions. Stable `1.x` releases also receive `latest`, major, and
minor tags.
