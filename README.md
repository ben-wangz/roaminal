# Roaminal

Roaminal is a self-hosted, browser-based persistent Bash terminal. The Go
service owns Linux PTYs and HTTP/WebSocket access; an in-process Node worker
maintains xterm.js shadow state so scrollback can be restored after a restart.

The MVP supports one service instance with multiple terminal tabs. It is a
Linux-container deployment and is tested with Google Chrome on desktop, tablet,
and phone-sized viewports. It intentionally has no file browser, host registry,
agent integration, native client, PWA manifest, service worker, or CDN assets.

## Local development

Prerequisites are Go 1.26.5, Node.js 24.13.1, npm, and a Linux PTY environment.
The worker and frontend each use their checked-in lockfile.

```sh
npm --prefix terminal-worker ci
npm --prefix web ci
npm --prefix terminal-worker test
npm --prefix terminal-worker run lint
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run build
rm -rf internal/webassets/dist
cp -a web/dist internal/webassets/dist
go test ./...
go vet ./...
go build ./cmd/roaminal
```

Start with an explicit password and terms acknowledgement:

```sh
ROAMINAL_ACCEPT_TERMS=true ROAMINAL_PASSWORD='change-this' go run ./cmd/roaminal
```

The default bind address is `127.0.0.1:9846`; the initial working directory is
`/workspace`. See [configuration](docs/configuration.md) for precedence and
validation, and [API](docs/api.md) for the HTTP and WebSocket contract.

## Containers and Kubernetes

Build and run with Podman only:

```sh
IMAGE=registry.example.invalid/roaminal:$(git rev-parse HEAD)
podman build --file Containerfile --tag "$IMAGE" .
podman run --rm --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -p 9846:9846 \
  -e ROAMINAL_ACCEPT_TERMS=true \
  -e ROAMINAL_PASSWORD='change-this' \
  -v roaminal-state:/home/roaminal/.roaminal \
  -v roaminal-workspace:/workspace \
  "$IMAGE"
podman push "$IMAGE"
```

The ordinary manifests in `deploy/kubernetes/` use one `Recreate` Deployment,
RWO state/workspace PVCs, a `ClusterIP` Service, and `/healthz` probes. Replace
the example image and Secret with deployment-specific values. The full rollout,
TLS, proxy timeout, PVC permission, and backup procedure is in
[deployment](docs/deployment.md).

## Acknowledgements

Roaminal's original inspiration came from
[Tabminal](https://github.com/Leask/Tabminal) by Leask Wong. We are grateful to
the project and its author for the idea and for providing an open reference
during MVP development. Tabminal is distributed under
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
