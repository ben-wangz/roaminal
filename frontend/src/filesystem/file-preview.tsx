import { Download, FileQuestion, LoaderCircle, Search, X } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { loadMetadata, readContent } from './filesystem-api';
import type { FileEntry, FileMetadata, FileSystemError, RootContext } from './filesystem-types';
import { viewerFor, viewerLabel } from './viewer-registry';

type Props = {
  instanceId: string;
  root: RootContext;
  entry: FileEntry | null;
  onClose: () => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  onRootChanged: () => void;
};

export function FilePreview({ instanceId, root, entry, onClose, onToast, onRootChanged }: Props) {
  const [metadata, setMetadata] = useState<FileMetadata | null>(null);
  const [data, setData] = useState<ArrayBuffer | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [source, setSource] = useState<string | null>(null);
  const [markdownSource, setMarkdownSource] = useState(false);
  const onRootChangedRef = useRef(onRootChanged);
  onRootChangedRef.current = onRootChanged;
  const entryPath = entry?.type === 'file' ? entry.relativePath : null;
  const rootRevision = root.revision;

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setMetadata(null);
    setData(null);
    setSource(null);
    setError('');
    setMarkdownSource(false);
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
  }, [entryPath, instanceId, rootRevision]);

  useEffect(() => () => { if (source) URL.revokeObjectURL(source); }, [source]);

  const kind = metadata ? viewerFor(metadata) : null;
  const text = useMemo(() => data ? new TextDecoder(metadata?.encoding || 'utf-8', { fatal: false }).decode(data) : '', [data, metadata?.encoding]);
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
    return <div className="filesystem-preview filesystem-preview-empty-state"><div className="filesystem-preview-body"><div className="filesystem-preview-empty"><FileQuestion size={25} aria-hidden="true" /><span>Select a file and double-click to preview.</span></div></div></div>;
  }
  return (
    <section className="filesystem-preview" aria-label="File preview">
      <header className="filesystem-preview-header">
        <div className="filesystem-preview-heading">
          <strong title={entry.absolutePath}>{entry.name}</strong>
          <small>{metadata ? `${viewerLabel(kind || 'raw')} · ${formatSize(metadata.size)}` : entry.relativePath}</small>
        </div>
        <div className="filesystem-preview-actions">
          {kind === 'markdown' && <button className="icon-button" type="button" onClick={() => setMarkdownSource((value) => !value)} title={markdownSource ? 'Rendered markdown' : 'Markdown source'} aria-label={markdownSource ? 'Rendered markdown' : 'Markdown source'}><Search size={16} aria-hidden="true" /></button>}
          <button className="icon-button" type="button" onClick={() => void download()} title="Download" aria-label="Download"><Download size={16} aria-hidden="true" /></button>
          <button className="icon-button" type="button" onClick={onClose} title="Close preview" aria-label="Close preview"><X size={16} aria-hidden="true" /></button>
        </div>
      </header>
      <div className="filesystem-preview-body">
        {loading && <div className="filesystem-loading"><LoaderCircle size={18} className="spin" aria-hidden="true" /> Loading preview</div>}
        {!loading && error && <div className="filesystem-error">{error}</div>}
        {!loading && !error && kind === 'image' && source && <img className="filesystem-image-viewer" src={source} alt={entry.name} />}
        {!loading && !error && kind === 'video' && source && <video className="filesystem-video-viewer" src={source} controls preload="metadata" />}
        {!loading && !error && kind === 'pdf' && source && <iframe className="filesystem-pdf-viewer" src={source} title={entry.name} />}
        {!loading && !error && (kind === 'text' || (kind === 'markdown' && markdownSource)) && <pre className="filesystem-text-viewer">{text}</pre>}
        {!loading && !error && kind === 'markdown' && !markdownSource && <MarkdownPreview value={text} />}
        {!loading && !error && kind === 'raw' && <div className="filesystem-raw-viewer"><FileQuestion size={24} aria-hidden="true" /><span>Binary preview is unavailable. Use Download to inspect this file.</span></div>}
      </div>
    </section>
  );
}

function MarkdownPreview({ value }: { value: string }) {
  const html = value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replace(/^### (.*)$/gm, '<h3>$1</h3>')
    .replace(/^## (.*)$/gm, '<h2>$1</h2>')
    .replace(/^# (.*)$/gm, '<h1>$1</h1>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/\n\n/g, '</p><p>')
    .replace(/\n/g, '<br />');
  return <article className="filesystem-markdown-viewer" dangerouslySetInnerHTML={{ __html: `<p>${html}</p>` }} />;
}

function formatSize(size: number | null): string {
  if (size === null) return 'size unavailable';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
