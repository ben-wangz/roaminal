import { useEffect, useRef, type RefObject } from 'react';
import { ChevronDown, FolderTree, Keyboard, Users } from 'lucide-react';
import { ConnectionSidebar } from './connection-sidebar';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ConnectionInstanceLayout, InstanceMovePlacement } from '../connections/connection-instance-groups';
import type { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ContextualMode } from '../input/contextual-keyboard-model';
import { VirtualKeyboardDock } from '../input/virtual-keyboard-dock';
import type { WorkspaceContent } from '../app/workspace-content';
import type { WorkspaceTool } from '../app/workspace-tool';
import { FileTreeToolSurface } from '../filesystem/file-tree-tool-surface';
import type { FileSystemWorkspaceState } from '../filesystem/use-filesystem-workspace';

type Props = {
  tool: WorkspaceTool;
  open: boolean;
  workspaceContent: WorkspaceContent;
  connections: ConnectionInstanceSummary[];
  layout: ConnectionInstanceLayout;
  loginSessionId: string;
  active: string | null;
  previewConnectionInstanceId: string | null;
  previewRuntime: TerminalPreviewRuntime | null;
  activeInstance: ConnectionInstanceSummary | null;
  activeRuntime: TerminalRuntime | null;
  contextualMode: ContextualMode;
  nativeKeyboardOpen: boolean;
  connectionToolButton: RefObject<HTMLButtonElement | null>;
  keyboardToolButton: RefObject<HTMLButtonElement | null>;
  filesToolButton: RefObject<HTMLButtonElement | null>;
  onCollapse: () => void;
  onAddConnection: () => void;
  onSelectConnection: (id: string) => void;
  onMoveConnectionInstance: (id: string, groupId: string, targetId: string | null, placement: InstanceMovePlacement) => Promise<void>;
  onReorderConnectionGroup: (id: string, targetId: string, placement: InstanceMovePlacement) => Promise<void>;
  onCreateConnectionGroup: (name: string) => Promise<boolean>;
  onRenameConnectionGroup: (id: string, name: string) => Promise<boolean>;
  onDeleteConnectionGroup: (id: string) => Promise<boolean>;
  onMoveConnectionGroupMembers: (id: string) => Promise<void>;
  onPreviewStart: (id: string) => void;
  onPreviewEnd: (id: string) => void;
  onOpenTerminal: (id: string) => void;
  onAgent: (id: string) => void;
  onOpenFileTree: (id: string) => void;
  filesystem: FileSystemWorkspaceState;
  onRename: (id: string) => void;
  onAutomaticTitle: (id: string) => void;
  onTerminate: (id: string) => void;
  onModeChange: (mode: ContextualMode) => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

function toolLabel(tool: WorkspaceTool): string {
  return tool === 'connections' ? 'Connections' : tool === 'keyboard' ? 'Virtual keyboard' : 'Files';
}

export function WorkspaceToolSurface({
  tool,
  open,
  workspaceContent,
  connections,
  layout,
  loginSessionId,
  active,
  previewConnectionInstanceId,
  previewRuntime,
  activeInstance,
  activeRuntime,
  contextualMode,
  nativeKeyboardOpen,
  connectionToolButton,
  keyboardToolButton,
  filesToolButton,
  onCollapse,
  onAddConnection,
  onSelectConnection,
  onMoveConnectionInstance,
  onReorderConnectionGroup,
  onCreateConnectionGroup,
  onRenameConnectionGroup,
  onDeleteConnectionGroup,
  onMoveConnectionGroupMembers,
  onPreviewStart,
  onPreviewEnd,
  onOpenTerminal,
  onAgent,
  onOpenFileTree,
  filesystem,
  onRename,
  onAutomaticTitle,
  onTerminate,
  onModeChange,
  onToast,
}: Props) {
  const surface = useRef<HTMLElement>(null);
  const collapseButton = useRef<HTMLButtonElement>(null);
  const previousOpen = useRef(false);
  const compact = window.matchMedia('(max-width: 800px)').matches;

  useEffect(() => {
    if (open && !previousOpen.current && compact && (tool === 'connections' || tool === 'files')) collapseButton.current?.focus();
    previousOpen.current = open;
  }, [compact, open, tool]);

  useEffect(() => {
    if (!open || !compact) return;
    const handleKeyboard = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        // An open menu/dialog owns Escape.  In particular, the Files context
        // menu is mounted inside this surface and closes itself on Escape;
        // collapsing the mobile surface here as well would make the focused
        // file row inert before keyboard context-menu shortcuts can run.
        const target = event.target;
        if (target instanceof Element && target.closest('[role="menu"], [role="dialog"]')) return;
        event.preventDefault();
        onCollapse();
        return;
      }
      if (event.key !== 'Tab' || !surface.current || (tool !== 'connections' && tool !== 'files')) return;
      const focusable = Array.from(surface.current.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'));
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!surface.current.contains(document.activeElement)) {
        event.preventDefault();
        first.focus();
      } else if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', handleKeyboard);
    return () => document.removeEventListener('keydown', handleKeyboard);
  }, [compact, onCollapse, open, tool]);

  const trigger = tool === 'connections' ? connectionToolButton : tool === 'keyboard' ? keyboardToolButton : filesToolButton;
  const handleCollapse = () => {
    onCollapse();
    window.requestAnimationFrame(() => trigger.current?.focus());
  };

  return (
    <>
      {open && compact && (tool === 'connections' || tool === 'files') && <button className="workspace-tool-backdrop" type="button" aria-label={`Close ${toolLabel(tool)}`} onClick={handleCollapse} />}
      <aside
        ref={surface}
        id="workspace-tool-surface"
        className={`workspace-tool-surface workspace-tool-${tool} ${open ? 'open' : 'closed'}`}
        aria-hidden={!open}
        inert={!open || undefined}
      >
        <header className="workspace-tool-header">
          <div className="workspace-tool-title">
            {tool === 'connections' ? <Users size={16} aria-hidden="true" /> : tool === 'keyboard' ? <Keyboard size={16} aria-hidden="true" /> : <FolderTree size={16} aria-hidden="true" />}
            <strong>{toolLabel(tool)}</strong>
          </div>
          <button ref={collapseButton} className="icon-button workspace-tool-collapse" type="button" onClick={handleCollapse} aria-label={`Collapse ${toolLabel(tool)}`} title={`Collapse ${toolLabel(tool)}`} aria-expanded={open} aria-controls="workspace-tool-surface">
            <ChevronDown aria-hidden="true" size={17} />
          </button>
        </header>
        {tool === 'connections' ? (
          <ConnectionSidebar
            connections={connections}
            layout={layout}
            loginSessionId={loginSessionId}
            active={active}
            previewConnectionInstanceId={previewConnectionInstanceId}
            previewRuntime={previewRuntime}
            onSelect={onSelectConnection}
            onMoveInstance={onMoveConnectionInstance}
            onReorderGroup={onReorderConnectionGroup}
            onCreateGroup={onCreateConnectionGroup}
            onRenameGroup={onRenameConnectionGroup}
            onDeleteGroup={onDeleteConnectionGroup}
            onMoveGroupMembers={onMoveConnectionGroupMembers}
            onPreviewStart={onPreviewStart}
            onPreviewEnd={onPreviewEnd}
            onOpenTerminal={onOpenTerminal}
            onAgent={onAgent}
            onOpenFileTree={onOpenFileTree}
            workspaceContent={workspaceContent}
            onRename={onRename}
            onAutomaticTitle={onAutomaticTitle}
            onTerminate={onTerminate}
            onAddConnection={onAddConnection}
          />
        ) : tool === 'keyboard' ? (
          <VirtualKeyboardDock
            instance={activeInstance}
            runtime={activeRuntime}
            mode={contextualMode}
            nativeKeyboardOpen={nativeKeyboardOpen}
            onModeChange={onModeChange}
            onToast={onToast}
          />
        ) : <FileTreeToolSurface instance={activeInstance} workspace={filesystem} onToast={onToast} />}
      </aside>
    </>
  );
}
