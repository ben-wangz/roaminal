# PW-REL-003: Roaminal restart, audit retirement, and persistent data

Priority: P1. Capabilities: Kubernetes control over the dedicated Roaminal
release and its PVC. Viewport: desktop. Run serially and never restart unrelated
workloads.

## Procedure and assertions

1. Log in with a stable configured password. Create local and SSH/tmux
   connections, write non-secret markers, a workspace file, a custom title, and
   SSH config/key fixtures. Wait for snapshots.
2. Restart only the dedicated Roaminal Pod through its Deployment and wait for
   the Helm release to become ready at the same Service URL. Record that
   `/api/version.bootId` changed.
3. The old browser detects the changed boot ID and reloads without an exception.
   Stable auth sessions remain valid because the password and state PVC did not
   change.
4. Under the current connection-instance layout, pre-restart live processes are
   not resurrected: startup retires their metadata/snapshots to audit and the
   active list is empty, so the manager opens. Do not assert that SSH
   ControlMaster or local Bash survives a Pod restart.
5. The workspace file, SSH config, SSH keys, auth-session record, and tmux add-on
   YAML still exist. The old active instance directories are gone and one audit
   copy per instance exists without being exposed in UI/API.
6. Start fresh local/SSH/tmux connections and verify full usability. A remote
   tmux server outside the Roaminal Pod may still retain its named session and
   should be attachable through a fresh SSH transport.
7. Repeat a graceful Helm rollout once and verify child process groups terminate
   within the configured grace period; no orphan processes keep the old Pod
   alive.

## Pass gate

Inspect only the dedicated PVC and redact snapshot contents. Fail on stale live
cards, duplicate audit entries, lost workspace/SSH/auth data, reload loops, or
any global diagnostics violation after readiness.
