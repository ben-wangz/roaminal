import { AuthSessionsDialog, type AuthSessionSummary } from '../auth/auth-session-ui';
import { Toast, type ToastKind, type ToastState } from '../ui/toast';
import { RenameTitleDialog, CloseConnectionDialog } from '../ui/connection-dialogs';
import { AgentDialog } from '../ui/agent-dialog';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type Dialog = { type: 'rename' | 'terminate' | 'agent'; connectionInstanceId: string } | { type: 'auth' } | null;

type Props = {
  toast: ToastState | null;
  dialog: Dialog;
  dialogConnection: ConnectionInstanceSummary | undefined;
  authSessions: AuthSessionSummary[];
  currentAuthSessionId: string;
  authSessionBusy: string | null;
  onShowToast: (message: string, kind?: ToastKind) => void;
  onRenameTitle: (id: string, title: string | null) => Promise<void>;
  onTerminateConnection: (id: string) => Promise<void>;
  onRevokeAuthSession: (id: string) => void;
  onLogoutOtherAuthSessions: () => void;
  onCloseDialog: () => void;
};

export function AppShellOverlays({
  toast,
  dialog,
  dialogConnection,
  authSessions,
  currentAuthSessionId,
  authSessionBusy,
  onShowToast,
  onRenameTitle,
  onTerminateConnection,
  onRevokeAuthSession,
  onLogoutOtherAuthSessions,
  onCloseDialog,
}: Props) {
  return (
    <>
      <Toast toast={toast} />
      {dialog?.type === 'rename' && dialogConnection && (
        <RenameTitleDialog
          connection={dialogConnection}
          onSave={(title) => onRenameTitle(dialogConnection.connectionInstanceId, title)}
          onClose={onCloseDialog}
        />
      )}
      {dialog?.type === 'terminate' && dialogConnection && (
        <CloseConnectionDialog
          connection={dialogConnection}
          onConfirm={() => onTerminateConnection(dialogConnection.connectionInstanceId)}
          onClose={onCloseDialog}
        />
      )}
      {dialog?.type === 'auth' && (
        <AuthSessionsDialog
          sessions={authSessions}
          currentId={currentAuthSessionId}
          busy={authSessionBusy}
          onRevoke={onRevokeAuthSession}
          onLogoutOthers={onLogoutOtherAuthSessions}
          onClose={onCloseDialog}
        />
      )}
      {dialog?.type === 'agent' && dialogConnection && (
        <AgentDialog
          connection={dialogConnection}
          onClose={onCloseDialog}
          onShowToast={onShowToast}
        />
      )}
    </>
  );
}
