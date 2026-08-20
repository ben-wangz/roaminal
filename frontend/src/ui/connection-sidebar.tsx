import { memo, useEffect, useRef } from 'react';
import { Bot, FolderOpen, GripVertical, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { ContextualKeyboard } from '../input/contextual-keyboard';
import { SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import type { ContextualMode } from '../input/contextual-keyboard-model';
import { ConnectionActions } from './connection-actions';
import { TerminalPreview, type TerminalPreviewRuntime } from '../terminal/terminal-preview';
import { useConnectionReorder } from './use-connection-reorder';

type Props = {
  id: string;
  connections: ConnectionInstanceSummary[];
  active: string | null;
  open: boolean;
  previewConnectionInstanceId: string | null;
  previewRuntime: TerminalPreviewRuntime | null;
  onToggle: () => void;
  onSelect: (id: string) => void;
  onReorder: (draggedID: string, targetID: string, placement: 'before' | 'after') => Promise<void>;
  onPreviewStart: (id: string) => void;
  onPreviewEnd: (id: string) => void;
  onUnavailableExtension: (name: 'Agent') => void;
  onOpenFileSystem: (id: string) => void;
  onRename: (id: string) => void;
  onAutomaticTitle: (id: string) => void;
  onTerminate: (id: string) => void;
  activeInstance: ConnectionInstanceSummary | null;
  activeRuntime: TerminalRuntime | null;
  contextualMode: ContextualMode;
  onContextualModeChange: (mode: ContextualMode) => void;
};

export function shortConnectionId(id: string): string {
  const part = id.split('-').pop();
  return part && part.length >= 12 ? part.slice(-12) : id.slice(0, 12);
}

export function sinceLabel(createdAt: string): string {
  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) return 'Unknown';
  const pad = (value: number) => String(value).padStart(2, '0');
  const hour = date.getHours();
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(hour % 12 || 12)}:${pad(date.getMinutes())} ${hour >= 12 ? 'PM' : 'AM'}`;
}

function connectionStateLabel(connection: ConnectionInstanceSummary): string {
  if (connection.attention) return 'Activity waiting';
  if (connection.purpose === 'ssh_key_generation') return 'SSH key generation';
  return connection.type === 'ssh' ? 'SSH connection' : 'Local connection';
}

function connectionPathLabel(connection: ConnectionInstanceSummary): string | null {
  if (connection.purpose === 'ssh_key_generation') return `TARGET: ${connection.title || 'key'}`;
  const cwd = connection.cwd?.trim();
  return cwd ? `PWD: ${cwd}` : null;
}

function canPreview(): boolean {
  return window.matchMedia('(pointer: fine)').matches && !window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches;
}

export const ConnectionSidebar = memo(function ConnectionSidebar({
  id,
  connections,
  active,
  open,
  previewConnectionInstanceId,
  previewRuntime,
  onToggle,
  onSelect,
  onReorder,
  onPreviewStart,
  onPreviewEnd,
  onUnavailableExtension,
  onOpenFileSystem,
  onRename,
  onAutomaticTitle,
  onTerminate,
  activeInstance,
  activeRuntime,
  contextualMode,
  onContextualModeChange,
}: Props) {
  const aside = useRef<HTMLElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const mounted = useRef(false);
  const {
    clearDrag, draggedConnectionInstanceId, dragLeave, dragOver, drop, dropTarget, moveWithKeyboard, reorderPending, startDrag,
  } = useConnectionReorder({ connections, onReorder, onPreviewEnd });
  useEffect(() => {
    if (mounted.current && open) toggle.current?.focus();
    mounted.current = true;
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const handleKeyboard = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches) {
        event.preventDefault();
        onToggle();
        return;
      }
      if (event.key !== 'Tab' || !window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches || !aside.current) return;
      const focusable = Array.from(
        aside.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      );
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!aside.current.contains(document.activeElement)) {
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
  }, [onToggle, open]);

  return (
    <>
      {open && <button className="sidebar-backdrop" type="button" aria-label="Close sidebar" onClick={onToggle} />}
      <aside
        ref={aside}
        id={id}
        className={`sidebar ${open ? 'open' : 'closed'}`}
        aria-hidden={!open}
        inert={!open || undefined}
      >
        <div className="sidebar-header">
          <div className="brand-mark small">
            r<span>&gt;</span>
          </div>
          <strong>Roaminal</strong>
          <button
            ref={toggle}
            className="icon-button sidebar-toggle"
            type="button"
            onClick={onToggle}
            aria-label="Toggle sidebar"
            title="Toggle sidebar"
            aria-expanded={open}
            aria-controls={id}
          >
            {open ? <PanelLeftClose aria-hidden="true" size={18} /> : <PanelLeftOpen aria-hidden="true" size={18} />}
          </button>
        </div>
        <div className="connection-list">
          {connections.map((connection) => {
            const previewing = previewConnectionInstanceId === connection.connectionInstanceId && previewRuntime;
            const pathLabel = connectionPathLabel(connection);
            const startPreview = () => {
              if (!draggedConnectionInstanceId && !reorderPending && canPreview()) onPreviewStart(connection.connectionInstanceId);
            };
            const stopPreview = () => onPreviewEnd(connection.connectionInstanceId);
            const dropPlacement = dropTarget?.id === connection.connectionInstanceId ? dropTarget.placement : null;
            return (
              <article
                className={`connection-card ${connection.connectionInstanceId === active ? 'active' : ''} ${connection.attention ? 'attention' : ''} ${previewing ? 'previewing' : ''} ${draggedConnectionInstanceId === connection.connectionInstanceId ? 'dragging' : ''} ${dropPlacement ? `drop-${dropPlacement}` : ''}`}
                data-connection-id={connection.connectionInstanceId}
                key={connection.connectionInstanceId}
                onMouseEnter={startPreview}
                onMouseLeave={stopPreview}
                onClick={() => onSelect(connection.connectionInstanceId)}
                onDragOver={(event) => dragOver(event, connection.connectionInstanceId)}
                onDragLeave={(event) => dragLeave(event, connection.connectionInstanceId)}
                onDrop={(event) => drop(event, connection.connectionInstanceId)}
                onFocus={(event) => {
                  if (!event.currentTarget.contains(event.relatedTarget as Node | null)) startPreview();
                }}
                onBlur={(event) => {
                  if (!event.currentTarget.contains(event.relatedTarget as Node | null)) stopPreview();
                }}
              >
                <div className="connection-card-preview">
                  {previewing && <TerminalPreview runtime={previewRuntime} />}
                </div>
                <div className="connection-card-overlay">
                  <button
                    className="connection-select"
                    type="button"
                    onClick={() => onSelect(connection.connectionInstanceId)}
                    aria-current={connection.connectionInstanceId === active ? 'page' : undefined}
                    title={connection.connectionInstanceId}
                  >
                    <span className="connection-indicator" />
                    <span className="connection-title-wrap">
                      <b>{connection.title || 'Connection'}</b>
                      <small>{connectionStateLabel(connection)}</small>
                    </span>
                  </button>
                  <div className="connection-metadata">
                    <span>ID: {shortConnectionId(connection.connectionInstanceId)}</span>
                    {pathLabel && (
                      <span className="connection-path" title={connection.cwd}>
                        {pathLabel}
                      </span>
                    )}
                    <time dateTime={connection.createdAt} title={connection.createdAt}>
                      SINCE: {sinceLabel(connection.createdAt)}
                    </time>
                  </div>
                </div>
                <div className="connection-actions" aria-label="Connection extensions and actions">
                  <button
                    className="connection-drag-handle"
                    type="button"
                    draggable={!reorderPending}
                    aria-label={`Reorder ${connection.title || 'connection'}`}
                    title="Reorder connection"
                    onClick={(event) => event.stopPropagation()}
                    onDragStart={(event) => startDrag(event, connection.connectionInstanceId)}
                    onDragEnd={clearDrag}
                    onKeyDown={(event) => moveWithKeyboard(event, connection.connectionInstanceId)}
                  >
                    <GripVertical aria-hidden="true" size={15} />
                  </button>
                  <button
                    className="extension-button"
                    type="button"
                    aria-label="Agent extension"
                    aria-disabled="true"
                    title="Agent extension unavailable"
                    onClick={(event) => {
                      event.stopPropagation();
                      onUnavailableExtension('Agent');
                    }}
                  >
                    <Bot aria-hidden="true" size={15} />
                  </button>
                  <button
                    className="extension-button"
                    type="button"
                    aria-label="Files extension"
                    title="Open FileSystem"
                    onClick={(event) => {
                      event.stopPropagation();
                      onOpenFileSystem(connection.connectionInstanceId);
                    }}
                  >
                    <FolderOpen aria-hidden="true" size={15} />
                  </button>
                  <ConnectionActions
                    connection={connection}
                    onRename={() => onRename(connection.connectionInstanceId)}
                    onAutomaticTitle={() => onAutomaticTitle(connection.connectionInstanceId)}
                    onTerminate={() => onTerminate(connection.connectionInstanceId)}
                  />
                </div>
              </article>
            );
          })}
        </div>
        <ContextualKeyboard
          instance={activeInstance}
          runtime={activeRuntime}
          mode={contextualMode}
          onModeChange={onContextualModeChange}
        />
        <div className="sidebar-footer">Connection workspace</div>
      </aside>
    </>
  );
});
