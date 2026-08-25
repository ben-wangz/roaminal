import { useCallback, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { api, login } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ConnectionView } from './connection-view';
import { reconcileConnections } from './connection-view';
import type { ToastKind } from '../ui/toast';
import { ConnectionInstanceController } from '../connections/connection-instance-controller';

type DisposableRuntimeRef = MutableRefObject<{ dispose(): void } | null>;

type Params = {
  setAuth: Dispatch<SetStateAction<AuthState | null>>;
  setError: Dispatch<SetStateAction<string>>;
  controller: ConnectionInstanceController;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setActiveView: (next: ConnectionView) => void;
  setDialog: Dispatch<SetStateAction<{ type: 'rename' | 'terminate' | 'agent'; connectionInstanceId: string } | { type: 'auth' } | null>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  mainRuntime: MutableRefObject<TerminalRuntime | null>;
  previewRuntimeRef: DisposableRuntimeRef;
  viewRef: MutableRefObject<ConnectionView>;
  showToast: (message: string, kind?: ToastKind) => void;
};

export function useConnectionLifecycleActions({
  setAuth,
  setError,
  controller,
  setCurrentRuntime,
  setActiveView,
  setDialog,
  setPreviewConnectionInstanceId,
  setSearch,
  mainRuntime,
  previewRuntimeRef,
  viewRef,
  showToast,
}: Params) {
  const updateTitle = useCallback(async (id: string, title: string | null) => {
    const updated = await api<ConnectionInstanceSummary>(`/connection-instances/${id}/title`, {
      method: 'PATCH',
      body: JSON.stringify({ title }),
    });
    controller.setConnections((current) => current.map((connection) => connection.connectionInstanceId === id ? updated : connection));
    setDialog(null);
  }, [controller, setDialog]);

  const resetTitle = useCallback(async (id: string) => {
    try {
      await updateTitle(id, null);
    } catch (err) {
      showToast((err as Error).message, 'error');
    }
  }, [showToast, updateTitle]);

  const terminateConnection = useCallback(async (id: string) => {
    try {
      controller.markRevision();
      if (mainRuntime.current?.connectionInstanceId === id) {
        mainRuntime.current.dispose();
        mainRuntime.current = null;
        setCurrentRuntime(null);
      }
      previewRuntimeRef.current?.dispose();
      previewRuntimeRef.current = null;
      setPreviewConnectionInstanceId(null);
      await api(`/connection-instances/${id}`, { method: 'DELETE' });
      controller.setConnections((current) => {
        const next = current.filter((connection) => connection.connectionInstanceId !== id);
        setActiveView(reconcileConnections(next, viewRef.current, current.map((connection) => connection.connectionInstanceId)));
        return next;
      });
      setDialog(null);
      setSearch(false);
      setPreviewConnectionInstanceId(null);
    } catch (err) {
      showToast((err as Error).message, 'error');
    }
  }, [controller, mainRuntime, previewRuntimeRef, setActiveView, setCurrentRuntime, setDialog, setPreviewConnectionInstanceId, setSearch, showToast, viewRef]);

  const onLogin = useCallback(async (password: string) => {
    try {
      setAuth(await login(password));
      setError('');
    } catch (err) {
      setError((err as Error).message);
    }
  }, [setAuth, setError]);

  return { updateTitle, resetTitle, terminateConnection, onLogin };
}
