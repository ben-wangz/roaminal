import { Toast, type ToastKind, type ToastState } from '../ui/toast';
import { RenameTitleDialog, CloseConnectionDialog } from '../ui/connection-dialogs';
import { AgentDialog } from '../ui/agent-dialog';
import { AddConnectionDialog } from '../ui/add-connection-dialog';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type Dialog = { type: 'rename' | 'terminate' | 'agent'; connectionInstanceId: string } | { type: 'add-connection' } | null;

type Props = {
  toast: ToastState | null;
  dialog: Dialog;
  dialogConnection: ConnectionInstanceSummary | undefined;
  onShowToast: (message: string, kind?: ToastKind) => void;
  onRenameTitle: (id: string, title: string | null) => Promise<void>;
  onTerminateConnection: (id: string) => Promise<void>;
  onCloseDialog: () => void;
  connections: ConnectionInstanceSummary[];
  onCreateConnection: (definitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<boolean>;
  onManageNotifications: (connection: ConnectionInstanceSummary) => void;
};

export function AppShellOverlays({
  toast,
  dialog,
  dialogConnection,
  onShowToast,
  onRenameTitle,
  onTerminateConnection,
  onCloseDialog,
  connections,
  onCreateConnection,
  onManageNotifications,
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
      {dialog?.type === 'add-connection' && (
        <AddConnectionDialog
          connections={connections}
          onCreateConnection={onCreateConnection}
          onClose={onCloseDialog}
        />
      )}
      {dialog?.type === 'agent' && dialogConnection && (
        <AgentDialog
          connection={dialogConnection}
          onClose={onCloseDialog}
          onShowToast={onShowToast}
          onManageNotifications={() => onManageNotifications(dialogConnection)}
        />
      )}
    </>
  );
}
