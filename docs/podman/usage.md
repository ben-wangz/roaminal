# Podman Usage

Run the published Roaminal runtime image as one standalone container on a Linux
host. This does not require Kubernetes or a source build. For a Helm deployment,
see [deployment](../deployment.md).

## Requirements

- Podman on a Linux host.
- A published Roaminal runtime image tag. Runtime image versions are independent
  of Helm Chart versions.
- A `linux/amd64` host for the current image build. The Containerfile compiles
  the backend for `linux/amd64`; it does not currently produce a native ARM64
  backend image.

## Pull the image

Choose a runtime version and pull its image from GitHub Container Registry:

```sh
export ROAMINAL_VERSION='<runtime-version>'
export ROAMINAL_IMAGE="ghcr.io/ben-wangz/roaminal:${ROAMINAL_VERSION}"
podman pull "$ROAMINAL_IMAGE"
```

Authenticate with `podman login ghcr.io` first if the package is private to your
account.

## Configure and start

Set a stable password and acknowledge the terms. Keep the environment file
private; changing the password invalidates existing login sessions.

```sh
install -d -m 700 "$HOME/.config/roaminal"
umask 077
read -r -s -p 'Roaminal password: ' ROAMINAL_PASSWORD
printf '\n'
printf 'ROAMINAL_ACCEPT_TERMS=true\nROAMINAL_PASSWORD=%s\n' "$ROAMINAL_PASSWORD" \
  > "$HOME/.config/roaminal/roaminal.env"
unset ROAMINAL_PASSWORD
chmod 600 "$HOME/.config/roaminal/roaminal.env"
```

Start the container with persistent volumes for state, workspace, SSH files,
and the image-preview cache:

```sh
podman run -d \
  --name roaminal \
  --restart=unless-stopped \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -p 127.0.0.1:9846:9846 \
  --env-file "$HOME/.config/roaminal/roaminal.env" \
  --volume roaminal-state:/home/roaminal/.roaminal \
  --volume roaminal-workspace:/workspace \
  --volume roaminal-ssh:/home/roaminal/.ssh \
  --volume roaminal-image-previews:/var/cache/roaminal/filesystem-image-previews \
  "$ROAMINAL_IMAGE"
```

Open `http://127.0.0.1:9846`. The first login requires TOTP enrollment. Check
startup output with `podman logs -f roaminal`.

The image runs as UID/GID 1000. Persistent volumes must be writable by that
identity. If a new named volume has an ownership problem, Podman's `:U` mount
option can set its ownership to the container user. It recursively changes
ownership, so check existing volume contents before using it. On SELinux hosts,
add `:Z` to mounts that need a private container label.

## Host files and SSH

Named volumes are managed by Podman and are not host directories. To use a host
project directory as the workspace, replace the workspace volume with a bind
mount, for example:

```sh
--volume "$HOME/projects:/workspace:Z"
```

Make sure the mounted directory is writable by UID/GID 1000 in the container.
Connection definitions and SSH keys are read from
`/home/roaminal/.ssh`. You can manage them in the mounted SSH volume or mount a
host SSH directory there. A read-only SSH mount permits connections but
prevents edits and key operations through Roaminal.

The local shell runs inside the container. It sees the image's installed tools
and mounted files, not the host filesystem or host-installed tools unless you
explicitly mount them. SSH connections run from the container's network
namespace.

## Operations

View logs and stop the container with:

```sh
podman logs -f roaminal
podman stop roaminal
```

To upgrade, pull the new runtime image, stop and remove the old container, then
run it again with the same environment file and volumes. Removing the container
does not remove named volumes. Use a systemd service or Podman Quadlet when the
container must start with the host.

Stop Roaminal before backing up the `roaminal-state`, `roaminal-workspace`, and
`roaminal-ssh` volumes. The preview cache is disposable. Preserve file ownership
and permissions when restoring; see [backup and recovery](../backup-recovery.md).

## Network and lifecycle

The example binds the web port to loopback. For remote access, put Roaminal
behind a TLS reverse proxy that preserves WebSocket upgrades and allows at
least one hour for read and send timeouts. Do not expose worker or runtime
sockets.

Run one container only. A stop, restart, or image upgrade interrupts active
connection instances, SSH transports, and tmux processes; saved state does not
reopen them. The container does not provide high availability or automatic
volume backups.
