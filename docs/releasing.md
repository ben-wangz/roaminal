# Releasing Roaminal

Roaminal has one release version because the container ships the backend,
frontend, and terminal worker together. ForgeKit `v0.6.1` is pinned by
`setup/forgekit.sh` and reads/writes the canonical `container/VERSION` through
`version-control.yaml`.

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
```

The normal version is suitable for a release tag. The Git version appends the
current commit and dirty-worktree state and is useful for development image
tags only.

## Bump and review

Use ForgeKit for the product version mutation. Choose exactly one increment:

```sh
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version bump container roaminal patch
# or: minor
# or: major
```

Inspect the version-only diff before staging anything else:

```sh
git diff -- container/VERSION
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" version get roaminal
```

Run the same gate used by CI, then commit the version change. Do not edit
`container/VERSION` by hand, and do not bump the private npm module versions.

```sh
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" lint \
  --config "$PROJECT_ROOT/lint.yaml"
git add container/VERSION
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
ROAMINAL_VERSION="$(VERSION_JSON="$VERSION_JSON" node -p 'JSON.parse(process.env.VERSION_JSON).data.app.value')"
CONTAINER_REGISTRY=registry.example.invalid \
IMAGE_NAME=roaminal \
BUILD_ARG_ROAMINAL_VERSION="$ROAMINAL_VERSION" \
"$FORGEKIT_BIN" --project-root "$PROJECT_ROOT" publish container build \
  --container-dir container \
  --module roaminal \
  --context "$PROJECT_ROOT" \
  --no-push
```

This refactor does not introduce automated registry publication. Pushing an
image, signing it, and deploying it remain explicit release-owner actions after
the version commit and CI checks have passed.
