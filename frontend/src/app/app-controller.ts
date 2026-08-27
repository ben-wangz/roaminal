import { useCallback, useRef, useSyncExternalStore, type SetStateAction } from 'react';
import { loadStoredConnection, type ConnectionView } from './connection-view';
import type { AppPage } from './app-state';
import type { Dialog } from './app-shell-view';
import { SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import type { WorkspaceTool } from './workspace-tool';

export type AppControllerState = {
  view: ConnectionView;
  page: AppPage;
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  previewConnectionInstanceId: string | null;
  search: boolean;
  dialog: Dialog;
};

type Listener = () => void;

class AppController {
  private state: AppControllerState;
  private readonly listeners = new Set<Listener>();

  constructor(initial: AppControllerState) {
    this.state = initial;
  }

  getSnapshot = (): AppControllerState => this.state;

  subscribe = (listener: Listener): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  setState(update: (current: AppControllerState) => AppControllerState): void {
    const next = update(this.state);
    if (next === this.state) return;
    this.state = next;
    for (const listener of this.listeners) listener();
  }

  setView(view: ConnectionView): void { this.setState((current) => ({ ...current, view })); }
  setPage(page: AppPage): void { this.setState((current) => ({ ...current, page })); }
  setWorkspaceTool(workspaceTool: WorkspaceTool): void { this.setState((current) => ({ ...current, workspaceTool })); }
  setWorkspaceToolOpen(workspaceToolOpen: boolean): void { this.setState((current) => ({ ...current, workspaceToolOpen })); }
  setPreviewConnectionInstanceId(previewConnectionInstanceId: string | null): void {
    this.setState((current) => ({ ...current, previewConnectionInstanceId }));
  }
  setSearch(search: boolean): void { this.setState((current) => ({ ...current, search })); }
  setDialog(dialog: Dialog): void { this.setState((current) => ({ ...current, dialog })); }
}

export function createAppController(): AppController {
  const initialView = loadStoredConnection(typeof window === 'undefined' ? null : window.localStorage);
  const workspaceToolOpen = typeof window === 'undefined' || !window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches;
  return new AppController({
    view: initialView,
    page: 'connections',
    workspaceTool: 'connections',
    workspaceToolOpen,
    previewConnectionInstanceId: null,
    search: false,
    dialog: null,
  });
}

export function useAppController() {
  const controllerRef = useRef<AppController | null>(null);
  if (!controllerRef.current) controllerRef.current = createAppController();
  const controller = controllerRef.current;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const viewRef = useRef(state.view);
  viewRef.current = state.view;
  const setView = useCallback((view: ConnectionView) => controller.setView(view), [controller]);
  const setField = useCallback(<K extends keyof AppControllerState>(key: K, next: SetStateAction<AppControllerState[K]>) => {
    controller.setState((current) => ({
      ...current,
      [key]: typeof next === 'function'
        ? (next as (value: AppControllerState[K]) => AppControllerState[K])(current[key])
        : next,
    }));
  }, [controller]);
  const setPage = useCallback((next: SetStateAction<AppPage>) => setField('page', next), [setField]);
  const setWorkspaceTool = useCallback((next: SetStateAction<WorkspaceTool>) => setField('workspaceTool', next), [setField]);
  const setWorkspaceToolOpen = useCallback((next: SetStateAction<boolean>) => setField('workspaceToolOpen', next), [setField]);
  const setPreviewConnectionInstanceId = useCallback((next: SetStateAction<string | null>) => setField('previewConnectionInstanceId', next), [setField]);
  const setSearch = useCallback((next: SetStateAction<boolean>) => setField('search', next), [setField]);
  const setDialog = useCallback((next: SetStateAction<Dialog>) => setField('dialog', next), [setField]);
  return {
    state,
    controller,
    viewRef,
    setActiveView: setView,
    setView: (next: SetStateAction<ConnectionView>) => setField('view', next),
    setPage,
    setWorkspaceTool,
    setWorkspaceToolOpen,
    setPreviewConnectionInstanceId,
    setSearch,
    setDialog,
  };
}
