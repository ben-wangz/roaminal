import { useEffect, type MutableRefObject } from 'react';
import type { AuthState } from '../auth/auth-storage';
import { saveStoredConnection, type ConnectionView } from './connection-view';
import { ConnectionInstanceController } from '../connections/connection-instance-controller';

type DisposableRuntimeRef = MutableRefObject<{ dispose(): void } | null>;

type Params = {
  auth: AuthState | null;
  view: ConnectionView;
  viewRef: MutableRefObject<ConnectionView>;
  controller: ConnectionInstanceController;
  mainRuntime: DisposableRuntimeRef;
  previewRuntimeRef: DisposableRuntimeRef;
};

export function useAppShellLifecycle({
  auth,
  view,
  viewRef,
  controller,
  mainRuntime,
  previewRuntimeRef,
}: Params): void {
  useEffect(() => saveStoredConnection(window.localStorage, view), [view]);
  useEffect(() => {
    viewRef.current = view;
  }, [view, viewRef]);
  useEffect(() => {
    if (auth) return;
    controller.reset();
  }, [auth, controller]);
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
