# Releasing Roaminal

Roaminal releases one runtime image and one Helm Chart. ForgeKit `v0.6.1` is
pinned by `setup/forgekit.sh`; `version-control.yaml` registers the `roaminal`
Chart and its linked `roaminal-runtime` container. The Chart `version` and
runtime `container/VERSION` are separate version sources, while Chart
`appVersion` and `image.tag` are synchronized from the linked runtime.

## Inspect the version

Run these commands from any directory inside the checkout; the bootstrap script
locates the repository from its own path and stores the binary in ignored
`build/bin/forgekit`:

```sh
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
FORGEKIT_BIN="$(bash "$PROJECT_ROOT/setup/forgekit.sh")"
"$FORGEKIT_BIN" --version
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal --git
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal-runtime
```

The normal version is suitable for a release tag. The Git version appends the
current commit and dirty-worktree state and is useful for development image
tags only.

## Bump and review

Use ForgeKit for every version mutation. Choose exactly one increment for the
artifact that changed:

```sh
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump container roaminal-runtime patch
# or: minor / major

# Chart-only change (also syncs appVersion and image.tag):
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump chart roaminal patch --sync

# Runtime change followed by a Chart release:
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump container roaminal-runtime patch
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump chart roaminal patch --sync
```

Inspect the version-only diff before staging anything else:

```sh
git diff -- container/VERSION chart/Chart.yaml chart/values.yaml
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal
```

Run the same gate used by CI, then commit the version change. Do not edit
`container/VERSION` by hand, and do not bump the private npm module versions.

```sh
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" lint \
  --config "$PROJECT_ROOT/lint.yaml"
git add container/VERSION chart/Chart.yaml chart/values.yaml
git commit -m "release: Roaminal <version>"
```

## Tagging

Push the version commit and wait for the `lint` workflow on `main` to pass.
Create the annotated tag only after that successful workflow:

```sh
VERSION_JSON="$($FORGEKIT_BIN --project-root "$PROJECT_ROOT" --output json version get roaminal)"
VERSION="$(VERSION_JSON="$VERSION_JSON" node -p 'JSON.parse(process.env.VERSION_JSON).data.app.value')"
git tag -a "roaminal-v${VERSION}" -m "Roaminal ${VERSION}"
git push origin "roaminal-v${VERSION}"
```

Release tags always use `roaminal-v<semver>`. A tag must identify the exact
version commit that passed CI; do not tag a dirty worktree or a commit whose
lint workflow is still running.

## Container builds

Development builds use the ForgeKit Git version. A release build uses the
clean SemVer value so the image label, build argument, CLI output, and
`/api/version` agree:

```sh
VERSION_JSON="$($FORGEKIT_BIN --project-root "$PROJECT_ROOT" --output json version get roaminal)"
ROAMINAL_VERSION="$(VERSION_JSON="$VERSION_JSON" node -p 'JSON.parse(process.env.VERSION_JSON).data.app.linked.find((item) => item.name === "roaminal-runtime").value')"
CONTAINER_REGISTRY=registry.example.invalid \
IMAGE_NAME=roaminal \
BUILD_ARG_ROAMINAL_VERSION="$ROAMINAL_VERSION" \
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" publish container build \
  --container-dir container \
  --module roaminal-runtime \
  --context "$PROJECT_ROOT" \
  --no-push
```

The Chart can be packaged locally with:

```sh
helm lint chart --strict
helm package chart --destination dist/charts
```

The release workflow publishes Chart packages to the configured OCI registry
after a `roaminal-v<chart-version>` tag. Pushing an image, signing it, and
deploying it remain explicit release-owner actions after the version commit and
CI checks have passed.
