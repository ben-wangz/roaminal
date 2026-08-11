# Roaminal

Roaminal is a self-hosted, browser-based persistent Bash terminal. The Go
service owns Linux PTYs and HTTP/WebSocket access; an in-process Node worker
maintains xterm.js shadow state so scrollback can be restored after a restart.

The service supports one instance with multiple persistent terminals. It is a
Linux-container deployment and is tested with Google Chrome on desktop, tablet,
and phone-sized viewports. It intentionally has no file browser, host registry,
agent integration, native client, PWA manifest, service worker, or CDN assets.

## Local development

Prerequisites are Go 1.26.5, Node.js 24.13.1, npm, and a Linux PTY environment.
The worker and frontend each use their checked-in lockfile.

```sh
npm --prefix terminal-worker ci
npm --prefix frontend ci
npm --prefix terminal-worker test
npm --prefix terminal-worker run lint
npm --prefix frontend run typecheck
npm --prefix frontend run lint
npm --prefix frontend run build
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./cmd/roaminal
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
"$FORGEKIT_BIN" --project-root "$PWD" version get roaminal-runtime
```

Use `forgekit version bump` for patch, minor, or major releases. The complete
review, tagging, and container build procedure is in
[releasing](docs/releasing.md).

Start with an explicit password and terms acknowledgement:

```sh
ROAMINAL_ACCEPT_TERMS=true ROAMINAL_PASSWORD='change-this' go run ./backend/cmd/roaminal
```

The default bind address is `127.0.0.1:9846`; the initial working directory is
`/workspace`. See [configuration](docs/configuration.md) for precedence and
validation, and [API](docs/api.md) for the HTTP and WebSocket contract.

## Containers and Kubernetes

Build and run with Podman only:

```sh
IMAGE=registry.example.invalid/roaminal:$(git rev-parse HEAD)
podman build --file container/Containerfile --tag "$IMAGE" .
podman run --rm --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -p 9846:9846 \
  -e ROAMINAL_ACCEPT_TERMS=true \
  -e ROAMINAL_PASSWORD='change-this' \
  -v roaminal-state:/home/roaminal/.roaminal \
  -v roaminal-workspace:/workspace \
  "$IMAGE"
podman push "$IMAGE"
```

The Helm Chart in `chart/` is the deployment source of truth. It uses one
`Recreate` Deployment, one unified RWO PVC with `state/`, `workspace/`, and
`ssh/` directories, a `ClusterIP` Service, and `/healthz` probes. The former
`deploy/kubernetes/` directory contains only a migration pointer; it is not a
second configuration path. The full rollout, TLS, proxy timeout, PVC
permission, migration, and backup procedure is in [deployment](docs/deployment.md).

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
- [Release procedure](docs/releasing.md)
