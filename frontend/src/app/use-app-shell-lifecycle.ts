import { useEffect, type Dispatch, type MutableRefObject, type RefObject, type SetStateAction } from 'react';
import type { AuthState } from '../auth/auth-storage';
import { saveStoredConnection, type ConnectionView } from './connection-view';
import type { ConnectionInstanceLayout } from '../connections/connection-instance-groups';

type DisposableRuntimeRef = MutableRefObject<{ dispose(): void } | null>;

type Params = {
  auth: AuthState | null;
  view: ConnectionView;
  viewRef: MutableRefObject<ConnectionView>;
  connectionInstanceLayout: ConnectionInstanceLayout | null;
  connectionInstanceLayoutRef: MutableRefObject<ConnectionInstanceLayout | null>;
  pendingConnectionInstanceLayout: MutableRefObject<ConnectionInstanceLayout | null>;
  setConnectionInstanceLayout: Dispatch<SetStateAction<ConnectionInstanceLayout | null>>;
  sidebarOpen: boolean;
  virtualKeyboardOpen: boolean;
  sidebarOpenButton: RefObject<HTMLButtonElement | null>;
  mainRuntime: DisposableRuntimeRef;
  previewRuntimeRef: DisposableRuntimeRef;
};

export function useAppShellLifecycle({
  auth,
  view,
  viewRef,
  connectionInstanceLayout,
  connectionInstanceLayoutRef,
  pendingConnectionInstanceLayout,
  setConnectionInstanceLayout,
  sidebarOpen,
  virtualKeyboardOpen,
  sidebarOpenButton,
  mainRuntime,
  previewRuntimeRef,
}: Params): void {
  useEffect(() => saveStoredConnection(window.localStorage, view), [view]);
  useEffect(() => {
    viewRef.current = view;
  }, [view, viewRef]);
  useEffect(() => {
    connectionInstanceLayoutRef.current = connectionInstanceLayout;
  }, [connectionInstanceLayout, connectionInstanceLayoutRef]);
  useEffect(() => {
    if (auth) return;
    pendingConnectionInstanceLayout.current = null;
    connectionInstanceLayoutRef.current = null;
    setConnectionInstanceLayout(null);
  }, [auth, connectionInstanceLayoutRef, pendingConnectionInstanceLayout, setConnectionInstanceLayout]);
  useEffect(() => {
    if (!sidebarOpen && !virtualKeyboardOpen) sidebarOpenButton.current?.focus();
  }, [sidebarOpen, sidebarOpenButton, virtualKeyboardOpen]);
  useEffect(
    () => () => {
      mainRuntime.current?.dispose();
      mainRuntime.current = null;
      previewRuntimeRef.current?.dispose();
      previewRuntimeRef.current = null;
    },
    [mainRuntime, previewRuntimeRef],
  );
}
