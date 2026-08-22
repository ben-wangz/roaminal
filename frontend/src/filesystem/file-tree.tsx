import { ChevronDown, ChevronRight, File, FileCode2, FileImage, FileJson2, FileText, Film, Folder, Link2, RefreshCw } from 'lucide-react';
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
};

export function FileTree({ rootEntry, entries, showHidden, expanded, selected, loading, errorPaths, onToggle, onSelect, onOpen, onContextMenu, onRootContextMenu }: Props) {
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
            onClick={() => onSelect(entry)}
            onDoubleClick={() => onOpen(entry)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') { event.preventDefault(); onOpen(entry); }
              if (event.key === ' ') { event.preventDefault(); onSelect(entry); }
            }}
            onContextMenu={(event) => onContextMenu(event, entry)}
            title={entry.name}
          >
            {entry.type === 'directory' ? (
              <button className="filesystem-tree-chevron" type="button" onClick={(event) => { event.stopPropagation(); onToggle(entry); }} aria-label={isExpanded ? `Collapse ${entry.name}` : `Expand ${entry.name}`}>
                {isLoading ? <RefreshCw className="spin" size={13} aria-hidden="true" /> : isExpanded ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
              </button>
            ) : <span className="filesystem-tree-chevron-spacer" />}
            <EntryIcon entry={entry} />
            <span className="filesystem-tree-name">{entry.name}</span>
            {hasError && <span className="filesystem-tree-error" title="Unable to load">!</span>}
          </div>
          {entry.type === 'directory' && isExpanded && renderEntries(entry.relativePath, depth + 1)}
        </div>
      );
    })
  );
  return (
    <div className="filesystem-tree" role="tree" aria-label="Remote files">
      <div className={`filesystem-tree-root-row ${selected === '.' ? 'selected' : ''}`} role="treeitem" tabIndex={0} aria-selected={selected === '.'} onClick={() => onSelect(rootEntry)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onSelect(rootEntry); } }} onContextMenu={(event) => onRootContextMenu(event, rootEntry)}>
        <Folder size={15} aria-hidden="true" />
        <span>Root</span>
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
