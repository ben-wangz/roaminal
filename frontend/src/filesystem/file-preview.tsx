import { ArrowLeft, Download, FileQuestion, LoaderCircle, Search } from 'lucide-react';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { loadMetadata, readContent } from './filesystem-api';
import type { FileEntry, FileMetadata, FileSystemError, RootContext } from './filesystem-types';
import { MarkdownPreview } from './markdown-preview';
import { viewerFor, viewerLabel } from './viewer-registry';

type Props = {
  instanceId: string;
  root: RootContext;
  entry: FileEntry | null;
  onBackToTerminal: () => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  onRootChanged: () => void;
};

type ScrollPosition = {
  top: number;
  left: number;
};

const previewScrollPositions = new Map<string, ScrollPosition>();
const MAX_PREVIEW_SCROLL_POSITIONS = 100;

function savePreviewScrollPosition(key: string | null, target: HTMLElement | null): void {
  if (!key || !target) return;
  previewScrollPositions.delete(key);
  previewScrollPositions.set(key, { top: target.scrollTop, left: target.scrollLeft });
  while (previewScrollPositions.size > MAX_PREVIEW_SCROLL_POSITIONS) {
    const oldest = previewScrollPositions.keys().next().value;
    if (oldest === undefined) break;
    previewScrollPositions.delete(oldest);
  }
}

export function FilePreview({ instanceId, root, entry, onBackToTerminal, onToast, onRootChanged }: Props) {
  const [metadata, setMetadata] = useState<FileMetadata | null>(null);
  const [data, setData] = useState<ArrayBuffer | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [source, setSource] = useState<string | null>(null);
  const [markdownSource, setMarkdownSource] = useState(false);
  const previewBodyRef = useRef<HTMLDivElement>(null);
  const loadedIdentityRef = useRef<string | null>(null);
  const lastRestoredScrollKeyRef = useRef<string | null>(null);
  const onRootChangedRef = useRef(onRootChanged);
  onRootChangedRef.current = onRootChanged;
  const entryPath = entry?.type === 'file' ? entry.relativePath : null;
  const entryAbsolutePath = entry?.type === 'file' ? entry.absolutePath : null;
  const rootRevision = root.revision;
  const previewIdentity = entryAbsolutePath ? `${instanceId}\u0000${entryAbsolutePath}` : null;
  const kind = metadata ? viewerFor(metadata) : null;
  const text = useMemo(() => data ? new TextDecoder(metadata?.encoding || 'utf-8', { fatal: false }).decode(data) : '', [data, metadata?.encoding]);
  const scrollKey = previewIdentity
    ? `${previewIdentity}\u0000${kind === 'markdown' && !markdownSource ? 'rendered' : kind === 'markdown' ? 'source' : 'content'}`
    : null;

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    const samePreview = loadedIdentityRef.current === previewIdentity;
    if (!samePreview) {
      setMetadata(null);
      setData(null);
      setSource(null);
      setMarkdownSource(false);
    }
    loadedIdentityRef.current = previewIdentity;
    setError('');
    if (!entryPath) {
      setLoading(false);
      return undefined;
    }
    setLoading(true);
    void (async () => {
      try {
        const next = await loadMetadata(instanceId, entryPath, rootRevision, controller.signal);
        if (!active) return;
        setMetadata(next);
        const kind = viewerFor(next);
        if (kind === 'image' || kind === 'video' || kind === 'pdf') {
          const result = await readContent(instanceId, entryPath, rootRevision, undefined, true, controller.signal);
          if (!active) return;
          setSource(URL.createObjectURL(new Blob([result.data], { type: next.mimeType })));
        } else {
          const result = await readContent(instanceId, entryPath, rootRevision, undefined, false, controller.signal);
          if (!active) return;
          setData(result.data);
        }
      } catch (reason) {
        const error = (reason instanceof Error ? reason : new Error('Unable to read file')) as FileSystemError;
        if (active) {
          if (error.code === 'filesystem_root_changed' && error.root) onRootChangedRef.current();
          setError(error.message);
        }
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [entryPath, instanceId, previewIdentity, rootRevision]);

  useLayoutEffect(() => {
    const target = previewBodyRef.current;
    if (!target || !scrollKey) return undefined;
    const save = () => savePreviewScrollPosition(scrollKey, target);
    target.addEventListener('scroll', save, { passive: true });
    return () => {
      save();
      target.removeEventListener('scroll', save);
    };
  }, [scrollKey]);

  useLayoutEffect(() => {
    const target = previewBodyRef.current;
    if (!target || !scrollKey || loading) return;
    const position = previewScrollPositions.get(scrollKey);
    if (position) {
      target.scrollTop = position.top;
      target.scrollLeft = position.left;
    } else if (lastRestoredScrollKeyRef.current !== scrollKey) {
      target.scrollTop = 0;
      target.scrollLeft = 0;
    }
    lastRestoredScrollKeyRef.current = scrollKey;
  }, [data, loading, metadata, scrollKey, source]);

  useEffect(() => () => { if (source) URL.revokeObjectURL(source); }, [source]);

  const hasPreviewContent = Boolean(metadata && (data !== null || source !== null || kind === 'raw'));
  const download = async () => {
    if (!entry || !metadata) return;
    try {
      const result = await readContent(instanceId, entry.relativePath, root.revision, undefined, true);
      const url = URL.createObjectURL(new Blob([result.data], { type: metadata.mimeType }));
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = entry.name;
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (reason) {
      const error = (reason instanceof Error ? reason : new Error('Unable to download file')) as FileSystemError;
      if (error.code === 'filesystem_root_changed' && error.root) onRootChanged();
      onToast('Unable to download file.', 'error');
    }
  };

  if (!entry) {
    return <div className="filesystem-preview filesystem-preview-empty-state"><div ref={previewBodyRef} className="filesystem-preview-body"><div className="filesystem-preview-empty"><FileQuestion size={25} aria-hidden="true" /><span>Select a file and double-click to preview.</span></div></div></div>;
  }
  return (
    <section className="filesystem-preview" aria-label="File preview">
      <header className="filesystem-preview-header">
        <div className="filesystem-preview-heading">
          <strong title={entry.absolutePath}>{entry.name}</strong>
          <small>{metadata ? `${viewerLabel(kind || 'raw')} · ${formatSize(metadata.size)}` : entry.relativePath}</small>
        </div>
        <div className="filesystem-preview-actions">
          <button autoFocus className="icon-button file-preview-back-terminal" type="button" onClick={onBackToTerminal} title="Back to Terminal" aria-label="Back to Terminal" data-testid="file-preview-back-terminal"><ArrowLeft size={16} aria-hidden="true" /></button>
          {kind === 'markdown' && <button className="icon-button" type="button" onClick={() => setMarkdownSource((value) => !value)} title={markdownSource ? 'Rendered markdown' : 'Markdown source'} aria-label={markdownSource ? 'Rendered markdown' : 'Markdown source'}><Search size={16} aria-hidden="true" /></button>}
          <button className="icon-button" type="button" onClick={() => void download()} title="Download" aria-label="Download"><Download size={16} aria-hidden="true" /></button>
        </div>
      </header>
      <div ref={previewBodyRef} className="filesystem-preview-body">
        {!hasPreviewContent && loading && <div className="filesystem-loading"><LoaderCircle size={18} className="spin" aria-hidden="true" /> Loading preview</div>}
        {!hasPreviewContent && !loading && error && <div className="filesystem-error">{error}</div>}
        {hasPreviewContent && kind === 'image' && source && <img className="filesystem-image-viewer" src={source} alt={entry.name} />}
        {hasPreviewContent && kind === 'video' && source && <video className="filesystem-video-viewer" src={source} controls preload="metadata" />}
        {hasPreviewContent && kind === 'pdf' && source && <iframe className="filesystem-pdf-viewer" src={source} title={entry.name} />}
        {hasPreviewContent && (kind === 'text' || (kind === 'markdown' && markdownSource)) && <pre className="filesystem-text-viewer">{text}</pre>}
        {hasPreviewContent && kind === 'markdown' && !markdownSource && <MarkdownPreview value={text} />}
        {hasPreviewContent && kind === 'raw' && <div className="filesystem-raw-viewer"><FileQuestion size={24} aria-hidden="true" /><span>Binary preview is unavailable. Use Download to inspect this file.</span></div>}
        {hasPreviewContent && loading && <div className="filesystem-preview-refreshing" role="status"><LoaderCircle size={14} className="spin" aria-hidden="true" /> Refreshing preview</div>}
        {hasPreviewContent && error && <div className="filesystem-preview-refresh-error" role="alert">{error}</div>}
      </div>
    </section>
  );
}

function formatSize(size: number | null): string {
  if (size === null) return 'size unavailable';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
