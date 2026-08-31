# Roaminal

Roaminal is a self-hosted browser platform for local and SSH connection
instances. The Go service owns PTYs, OpenSSH transports, and HTTP/WebSocket
access; a Node child process maintains the xterm.js shadow state. Connection
definitions and SSH keys are read from `~/.ssh/config` and `~/.ssh/` rather
than copied into application storage. Optional tmux attachment, local and
remote monitor, audit artifacts, and browser refresh re-attachment are
supported.

Browser refresh persistence applies while the backend process is running. A
backend or Pod restart retires active instances; it does not restore PTYs,
SSH transports, or scrollback. Roaminal is a Linux-container deployment tested
with Chrome on desktop, tablet, and phone-sized viewports.

## Local development

Prerequisites are Go 1.26.5, Node.js 24.13.1, npm, and a Linux PTY environment.
The worker and frontend each use their checked-in lockfile. Run focused local
checks while iterating; GitHub Actions runs the full ForgeKit gate for pull
requests and `main` pushes.

Focused checks, when needed:

```sh
npm --prefix terminal-worker ci
npm --prefix frontend ci
npm --prefix terminal-worker test
npm --prefix frontend run typecheck
go -C backend test ./...
```

## Product versions and releases

Roaminal ships as one runtime image and one Helm Chart. ForgeKit owns the
runtime version in `container/VERSION` and the Chart version in
`chart/Chart.yaml`; the private `frontend/` and `terminal-worker/` manifests
stay at `0.0.0` and are not independently released. Bootstrap ForgeKit and
inspect the linked versions with:

```sh
FORGEKIT_BIN="$(bash ./setup/forgekit.sh)"
"$FORGEKIT_BIN" --project-root "$PWD" version get roaminal
"$FORGEKIT_BIN" --project-root "$PWD" version get roaminal --git
```

The JSON output includes the linked `roaminal-runtime` container metadata. Use
`forgekit version bump` for patch, minor, or major releases; do not call
`version get roaminal-runtime` or edit version fields by hand. The maintainer-only
review, tagging, and automated image/Chart publication procedure is in
[release automation](docs/releasing.md).

Start with an explicit password and terms acknowledgement:

```sh
ROAMINAL_ACCEPT_TERMS=true ROAMINAL_PASSWORD='change-this' go run ./backend/cmd/roaminal
```

The default bind address is `127.0.0.1:9846`; the initial working directory is
`/workspace`. See [configuration](docs/configuration.md) for precedence and
validation, and [API](docs/api.md) for the HTTP and WebSocket contract.

## Containers and Kubernetes

For local integration only, build and run with Podman. Release images are
published by the tag-triggered GitHub Actions; do not use this command to push
an artifact:

```sh
IMAGE=roaminal:dev-$(git rev-parse --short HEAD)
podman build --file container/Containerfile --tag "$IMAGE" .
podman run --rm --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -p 9846:9846 \
  -e ROAMINAL_ACCEPT_TERMS=true \
  -e ROAMINAL_PASSWORD='change-this' \
  -v roaminal-state:/home/roaminal/.roaminal \
  -v roaminal-workspace:/workspace \
  -v roaminal-ssh:/home/roaminal/.ssh \
  "$IMAGE"
```

The Helm Chart in `chart/` is the deployment source of truth. It uses one
`Recreate` Deployment, one unified RWO PVC with `state/`, `workspace/`, and
`ssh/` directories, a `ClusterIP` Service, and `/healthz` probes. The
`deploy/kubernetes/` directory contains the migration pointer and an optional
empty Helm values override; it is not a second Chart or deployment source. The full rollout, TLS, proxy timeout, PVC
permission, migration, and backup procedure is in [deployment](docs/deployment.md).

Install a published Chart release with Helm and the repository override file:

```sh
helm registry login ghcr.io
helm upgrade --install roaminal \
  oci://ghcr.io/ben-wangz/roaminal-charts/roaminal \
  --version '<chart-version>' \
  --namespace develop --create-namespace \
  --values deploy/kubernetes/values.yaml \
  --set auth.existingSecret=roaminal \
  --set app.acceptTerms=true
```

## Acknowledgements

Roaminal's original inspiration came from
[Tabminal](https://github.com/Leask/Tabminal) by Leask Wong. We are grateful to
the project and its author for the idea and for providing an open reference
during early product development. Tabminal is distributed under
the [MIT License](https://github.com/Leask/Tabminal/blob/main/LICENSE).

## License

Roaminal's core is licensed under the [Mozilla Public License 2.0](LICENSE).
The project follows an open-core model: future plugin SDK and plugin protocol
code will be licensed under [Apache-2.0](LICENSES/Apache-2.0.txt), while
separately distributed official plugins will use proprietary licenses. See the
[licensing policy](docs/licensing.md) for the exact boundaries and distribution
requirements.

## Project documents

- [API reference](docs/api.md)
- [Configuration](docs/configuration.md)
- [Security model](docs/security.md)
- [Deployment and rollout](docs/deployment.md)
- [Backup and recovery](docs/backup-recovery.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Licensing policy](docs/licensing.md)
- [Maintainer release automation](docs/releasing.md)
