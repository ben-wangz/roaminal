import { useCallback, useEffect, useRef } from 'react';
import { ChevronDown, ChevronRight, File, FileCode2, FileImage, FileJson2, FileText, Film, Folder, Link2, MoreVertical, RefreshCw } from 'lucide-react';
import { useMobileMode } from '../input/mobile-mode';
import type { FileEntry } from './filesystem-types';

type Props = {
  rootEntry: FileEntry;
  entries: Map<string, FileEntry[]>;
  showHidden: boolean;
  expanded: Set<string>;
  selected: string | null;
  loading: Set<string>;
  errorPaths: Set<string>;
  onToggle: (entry: FileEntry) => void;
  onSelect: (entry: FileEntry) => void;
  onOpen: (entry: FileEntry) => void;
  onContextMenu: (event: React.MouseEvent, entry: FileEntry) => void;
  onRootContextMenu: (event: React.MouseEvent, entry: FileEntry) => void;
  onOpenMenuAt: (entry: FileEntry, x: number, y: number) => void;
};

type PendingLongPress = {
  pointerId: number;
  x: number;
  y: number;
  startedAt: number;
  timer: number;
  cleanup: () => void;
};

export function FileTree({ rootEntry, entries, showHidden, expanded, selected, loading, errorPaths, onToggle, onSelect, onOpen, onContextMenu, onRootContextMenu, onOpenMenuAt }: Props) {
  const mobileMode = useMobileMode();
  const pendingLongPress = useRef<PendingLongPress | null>(null);
  const suppressUntil = useRef(0);
  const callbacks = useRef({ onSelect, onOpenMenuAt });
  callbacks.current = { onSelect, onOpenMenuAt };

  const cancelLongPress = useCallback(() => {
    const pending = pendingLongPress.current;
    if (!pending) return;
    window.clearTimeout(pending.timer);
    pending.cleanup();
    pendingLongPress.current = null;
  }, []);

  useEffect(() => () => cancelLongPress(), [cancelLongPress]);

  const beginLongPress = useCallback((event: React.PointerEvent, entry: FileEntry) => {
    if (!event.isPrimary || event.button !== 0 || (event.pointerType !== 'touch' && !mobileMode)) return;
    cancelLongPress();
    const { pointerId, clientX, clientY } = event;
    const onMove = (moveEvent: globalThis.PointerEvent) => {
      if (moveEvent.pointerId !== pointerId) return;
      if (Math.abs(moveEvent.clientX - clientX) > 10 || Math.abs(moveEvent.clientY - clientY) > 10) cancelLongPress();
    };
    const onEnd = (endEvent: globalThis.PointerEvent) => {
      if (endEvent.pointerId === pointerId) cancelLongPress();
    };
    const cleanup = () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onEnd);
      document.removeEventListener('pointercancel', onEnd);
      document.removeEventListener('lostpointercapture', onEnd);
    };
    let timer = 0;
    timer = window.setTimeout(() => {
      const pending = pendingLongPress.current;
      if (!pending || pending.pointerId !== pointerId) return;
      pendingLongPress.current = null;
      pending.cleanup();
      suppressUntil.current = Date.now() + 1000;
      callbacks.current.onSelect(entry);
      callbacks.current.onOpenMenuAt(entry, clientX, clientY);
    }, 550);
    pendingLongPress.current = { pointerId, x: clientX, y: clientY, startedAt: Date.now(), timer, cleanup };
    document.addEventListener('pointermove', onMove, { passive: true });
    document.addEventListener('pointerup', onEnd, { passive: true });
    document.addEventListener('pointercancel', onEnd, { passive: true });
    document.addEventListener('lostpointercapture', onEnd, { passive: true });
  }, [cancelLongPress, mobileMode]);

  const suppressSyntheticAction = () => Date.now() < suppressUntil.current;
  const select = (entry: FileEntry) => {
    if (!suppressSyntheticAction()) onSelect(entry);
  };
  const open = (entry: FileEntry) => {
    if (!suppressSyntheticAction()) onOpen(entry);
  };
  const contextMenu = (event: React.MouseEvent, entry: FileEntry) => {
    if (suppressSyntheticAction()) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    onContextMenu(event, entry);
  };
  const openMoreActions = (event: React.MouseEvent, entry: FileEntry) => {
    event.preventDefault();
    event.stopPropagation();
    const anchor = event.currentTarget.getBoundingClientRect();
    onOpenMenuAt(entry, anchor.right, anchor.bottom);
  };
  const openKeyboardMenu = (event: React.KeyboardEvent, entry: FileEntry) => {
    if (event.key !== 'ContextMenu' && !(event.key === 'F10' && event.shiftKey)) return;
    event.preventDefault();
    const anchor = event.currentTarget.getBoundingClientRect();
    onOpenMenuAt(entry, anchor.right, anchor.bottom);
  };

  const renderEntries = (parent: string, depth: number): React.ReactNode => (
    (entries.get(parent) || []).filter((entry) => showHidden || !entry.name.startsWith('.')).map((entry) => {
      const isExpanded = expanded.has(entry.relativePath);
      const isLoading = loading.has(entry.relativePath);
      const hasError = errorPaths.has(entry.relativePath);
      return (
        <div key={entry.relativePath}>
          <div
            className={`filesystem-tree-row ${selected === entry.relativePath ? 'selected' : ''}`}
            style={{ paddingLeft: `${10 + depth * 17}px` }}
            role="treeitem"
            tabIndex={0}
            aria-selected={selected === entry.relativePath}
            aria-expanded={entry.type === 'directory' ? isExpanded : undefined}
            onClick={() => select(entry)}
            onDoubleClick={() => open(entry)}
            onPointerDown={(event) => beginLongPress(event, entry)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') { event.preventDefault(); open(entry); }
              if (event.key === ' ') { event.preventDefault(); select(entry); }
              openKeyboardMenu(event, entry);
            }}
            onContextMenu={(event) => contextMenu(event, entry)}
            title={entry.name}
          >
            {entry.type === 'directory' ? (
              <button className="filesystem-tree-chevron" type="button" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); onToggle(entry); }} aria-label={isExpanded ? `Collapse ${entry.name}` : `Expand ${entry.name}`}>
                {isLoading ? <RefreshCw className="spin" size={13} aria-hidden="true" /> : isExpanded ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
              </button>
            ) : <span className="filesystem-tree-chevron-spacer" />}
            <EntryIcon entry={entry} />
            <span className="filesystem-tree-name">{entry.name}</span>
            {hasError && <span className="filesystem-tree-error" title="Unable to load">!</span>}
            <button
              className="filesystem-tree-more-actions"
              type="button"
              onPointerDown={(event) => event.stopPropagation()}
              onClick={(event) => openMoreActions(event, entry)}
              aria-label={`More actions for ${entry.name}`}
              title={`More actions for ${entry.name}`}
              aria-haspopup="menu"
            >
              <MoreVertical size={14} aria-hidden="true" />
            </button>
          </div>
          {entry.type === 'directory' && isExpanded && renderEntries(entry.relativePath, depth + 1)}
        </div>
      );
    })
  );
  return (
    <div className="filesystem-tree" role="tree" aria-label="Remote files">
      <div
        className={`filesystem-tree-root-row ${selected === '.' ? 'selected' : ''}`}
        role="treeitem"
        tabIndex={0}
        aria-selected={selected === '.'}
        onClick={() => select(rootEntry)}
        onPointerDown={(event) => beginLongPress(event, rootEntry)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select(rootEntry); }
          openKeyboardMenu(event, rootEntry);
        }}
        onContextMenu={(event) => { if (suppressSyntheticAction()) { event.preventDefault(); return; } onRootContextMenu(event, rootEntry); }}
        title={rootEntry.name}
      >
        <Folder size={15} aria-hidden="true" />
        <span>Root</span>
        <button
          className="filesystem-tree-more-actions"
          type="button"
          onPointerDown={(event) => event.stopPropagation()}
          onClick={(event) => openMoreActions(event, rootEntry)}
          aria-label="More actions for Root"
          title="More actions for Root"
          aria-haspopup="menu"
        >
          <MoreVertical size={14} aria-hidden="true" />
        </button>
      </div>
      {renderEntries('.', 0)}
    </div>
  );
}

function EntryIcon({ entry }: { entry: FileEntry }) {
  if (entry.type === 'directory') return <Folder className="filesystem-entry-icon directory" size={15} aria-hidden="true" />;
  if (entry.type === 'symlink') return <Link2 className="filesystem-entry-icon" size={15} aria-hidden="true" />;
  const lower = entry.name.toLowerCase();
  if (/\.(png|jpe?g|gif|webp|svg|avif)$/.test(lower)) return <FileImage className="filesystem-entry-icon" size={15} aria-hidden="true" />;
  if (/\.(mp4|webm|mov|ogv)$/.test(lower)) return <Film className="filesystem-entry-icon" size={15} aria-hidden="true" />;
  if (/\.(json|ya?ml|toml|xml)$/.test(lower)) return <FileJson2 className="filesystem-entry-icon" size={15} aria-hidden="true" />;
  if (/\.(go|ts|tsx|js|jsx|css|html|sh|py|rs|java|sql)$/.test(lower)) return <FileCode2 className="filesystem-entry-icon" size={15} aria-hidden="true" />;
  if (/\.(md|txt|log|pdf)$/.test(lower)) return <FileText className="filesystem-entry-icon" size={15} aria-hidden="true" />;
  return <File className="filesystem-entry-icon" size={15} aria-hidden="true" />;
}
