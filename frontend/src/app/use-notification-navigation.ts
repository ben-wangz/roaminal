import { useCallback } from 'react';
import type { AuthState } from '../auth/auth-storage';
import type { ToastKind } from '../ui/toast';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { WorkspaceTool } from './workspace-tool';
import { fetchMessages, type AgentMessage } from '../messages/message-api';
import { resolveMessageTarget } from '../messages/message-center';
import type { useMessages } from '../messages/use-messages';

type Params = {
  auth: AuthState | null;
  messageCenter: ReturnType<typeof useMessages>;
  connections: ConnectionInstanceSummary[];
  activeConnectionInstanceId: string | null;
  onOpenTerminal: (id: string) => void;
  setWorkspaceTool: (tool: WorkspaceTool) => void;
  setWorkspaceToolOpen: (open: boolean) => void;
  onToast: (message: string, kind?: ToastKind) => void;
};

export function useNotificationNavigation({
  auth,
  messageCenter,
  connections,
  activeConnectionInstanceId,
  onOpenTerminal,
  setWorkspaceTool,
  setWorkspaceToolOpen,
  onToast,
}: Params) {
  return useCallback(async (messageId: string) => {
    let message: AgentMessage | undefined = messageCenter.state.messages.find((item) => item.messageId === messageId);
    if (!message && auth) {
      try {
        const page = await fetchMessages(auth);
        message = page.messages.find((item) => item.messageId === messageId);
      } catch {
        // The durable Message Center remains the fallback when hydration fails.
      }
    }
    if (!message) {
      onToast('The connection for this message is no longer connected.', 'error');
      return;
    }
    await messageCenter.markRead(message.sequence);
    messageCenter.closePopover();
    const target = resolveMessageTarget(message, connections, activeConnectionInstanceId);
    if (!target.connectionInstanceId) {
      onToast('The connection for this message is no longer connected.', 'error');
      return;
    }
    setWorkspaceTool('connections');
    setWorkspaceToolOpen(!window.matchMedia('(max-width: 800px)').matches);
    onOpenTerminal(target.connectionInstanceId);
  }, [activeConnectionInstanceId, auth, connections, messageCenter, onOpenTerminal, onToast, setWorkspaceTool, setWorkspaceToolOpen]);
}
