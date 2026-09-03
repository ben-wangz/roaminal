import { Download, FileQuestion, LoaderCircle, ScanSearch, Search, SquareTerminal, ZoomIn, ZoomOut } from 'lucide-react';
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ChangeEvent, type PointerEvent as ReactPointerEvent } from 'react';
import { loadMetadata, readContent } from './filesystem-api';
import type { FileEntry, FileMetadata, FileSystemError, RootContext } from './filesystem-types';
import { formatSize, getPreviewScrollPosition, savePreviewScrollPosition } from './file-preview-helpers';
import { MarkdownPreview } from './markdown-preview';
import { viewerFor, viewerLabel } from './viewer-registry';

export type ImageDisplaySize = {
  width: number;
  height: number;
  scale: number;
};

type ImagePan = { x: number; y: number };
type ImageDrag = ImagePan & { pointerId: number; startX: number; startY: number };

const MIN_IMAGE_ZOOM = 25;
const MAX_IMAGE_ZOOM = 400;
const IMAGE_ZOOM_STEP = 25;

export function fitImageSize(
  naturalWidth: number,
  naturalHeight: number,
  availableWidth: number,
  availableHeight: number,
): ImageDisplaySize | null {
  if (![naturalWidth, naturalHeight, availableWidth, availableHeight].every((value) => Number.isFinite(value) && value > 0)) return null;
  const scale = Math.min(1, availableWidth / naturalWidth, availableHeight / naturalHeight);
  return {
    width: Math.max(1, Math.floor(naturalWidth * scale)),
    height: Math.max(1, Math.floor(naturalHeight * scale)),
    scale,
  };
}

type Props = {
  instanceId: string;
  root: RootContext;
  entry: FileEntry | null;
  onBackToTerminal: () => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  onRootChanged: () => void;
};

export function FilePreview({ instanceId, root, entry, onBackToTerminal, onToast, onRootChanged }: Props) {
  const [metadata, setMetadata] = useState<FileMetadata | null>(null);
  const [data, setData] = useState<ArrayBuffer | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [source, setSource] = useState<string | null>(null);
  const [imageVariant, setImageVariant] = useState<'preview' | 'original' | null>(null);
  const [originalLoading, setOriginalLoading] = useState(false);
  const [imageZoom, setImageZoom] = useState(100);
  const [imagePan, setImagePan] = useState<ImagePan>({ x: 0, y: 0 });
  const [imageDragging, setImageDragging] = useState(false);
  const [markdownSource, setMarkdownSource] = useState(false);
  const previewBodyRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);
  const [imageDisplaySize, setImageDisplaySize] = useState<ImageDisplaySize | null>(null);
  const loadedIdentityRef = useRef<string | null>(null);
  const lastRestoredScrollKeyRef = useRef<string | null>(null);
  const imageRequestControllerRef = useRef<AbortController | null>(null);
  const imageDragRef = useRef<ImageDrag | null>(null);
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

  const updateImageDisplaySize = () => {
    const image = imageRef.current;
    const target = previewBodyRef.current;
    if (!image || !target) return;
    const style = window.getComputedStyle(target);
    const horizontalPadding = parseFloat(style.paddingLeft) + parseFloat(style.paddingRight);
    const verticalPadding = parseFloat(style.paddingTop) + parseFloat(style.paddingBottom);
    const size = fitImageSize(
      image.naturalWidth,
      image.naturalHeight,
      target.clientWidth - horizontalPadding,
      target.clientHeight - verticalPadding,
    );
    if (size) setImageDisplaySize(size);
  };

  const handleImageZoomChange = (event: ChangeEvent<HTMLInputElement>) => {
    const next = Number(event.currentTarget.value);
    if (Number.isFinite(next)) setImageZoom(Math.min(MAX_IMAGE_ZOOM, Math.max(MIN_IMAGE_ZOOM, next)));
  };

  const handleImagePointerDown = (event: ReactPointerEvent<HTMLImageElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    imageDragRef.current = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, x: imagePan.x, y: imagePan.y };
    setImageDragging(true);
  };

  const handleImagePointerMove = (event: ReactPointerEvent<HTMLImageElement>) => {
    const drag = imageDragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    event.preventDefault();
    setImagePan({ x: drag.x + event.clientX - drag.startX, y: drag.y + event.clientY - drag.startY });
  };

  const finishImageDrag = (event?: ReactPointerEvent<HTMLImageElement>) => {
    const drag = imageDragRef.current;
    if (!drag || (event && drag.pointerId !== event.pointerId)) return;
    if (event?.currentTarget.hasPointerCapture?.(drag.pointerId)) event.currentTarget.releasePointerCapture?.(drag.pointerId);
    imageDragRef.current = null;
    setImageDragging(false);
  };

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    const samePreview = loadedIdentityRef.current === previewIdentity;
    if (!samePreview) {
      setMetadata(null);
      setData(null);
      setSource(null);
      setImageVariant(null);
      setOriginalLoading(false);
      setImageZoom(100);
      setImagePan({ x: 0, y: 0 });
      setImageDragging(false);
      imageDragRef.current = null;
      setImageDisplaySize(null);
      setMarkdownSource(false);
    }
    loadedIdentityRef.current = previewIdentity;
    setError('');
    if (!entryPath) {
      setLoading(false);
      return undefined;
    }
    setLoading(true);
    imageRequestControllerRef.current = controller;
    void (async () => {
      try {
        const next = await loadMetadata(instanceId, entryPath, rootRevision, controller.signal);
        if (!active) return;
        setMetadata(next);
        const kind = viewerFor(next);
        if (kind === 'image') {
          try {
            const result = await readContent(instanceId, entryPath, rootRevision, { variant: 'preview', signal: controller.signal });
            if (!active) return;
            setImageVariant('preview');
            setSource(URL.createObjectURL(new Blob([result.data], { type: result.response.headers.get('Content-Type') || 'image/webp' })));
          } catch (previewReason) {
            if (!active) return;
            const previewError = (previewReason instanceof Error ? previewReason : new Error('Unable to load image preview')) as FileSystemError;
            try {
              const result = await readContent(instanceId, entryPath, rootRevision, { variant: 'original', signal: controller.signal });
              if (!active) return;
              setImageZoom(100);
              setImagePan({ x: 0, y: 0 });
              setImageDragging(false);
              imageDragRef.current = null;
              setImageVariant('original');
              setSource(URL.createObjectURL(new Blob([result.data], { type: next.mimeType })));
            } catch (originalReason) {
              if (!active) return;
              const originalError = (originalReason instanceof Error ? originalReason : previewError) as FileSystemError;
              if (originalError.code === 'filesystem_root_changed' && originalError.root) onRootChangedRef.current();
              setError(originalError.message || previewError.message);
            }
          }
        } else if (kind === 'video' || kind === 'pdf') {
          const result = await readContent(instanceId, entryPath, rootRevision, { variant: 'original', signal: controller.signal });
          if (!active) return;
          setSource(URL.createObjectURL(new Blob([result.data], { type: next.mimeType })));
        } else {
          const result = await readContent(instanceId, entryPath, rootRevision, { signal: controller.signal });
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
        if (imageRequestControllerRef.current === controller) imageRequestControllerRef.current = null;
      }
    })();
    return () => {
      active = false;
      controller.abort();
      imageRequestControllerRef.current?.abort();
      imageRequestControllerRef.current = null;
    };
  }, [entryPath, instanceId, previewIdentity, rootRevision]);

  useEffect(() => {
    if (kind !== 'image' || !source) {
      setImageDisplaySize(null);
      return undefined;
    }
    const image = imageRef.current;
    const target = previewBodyRef.current;
    if (!image || !target) return undefined;
    const update = () => updateImageDisplaySize();
    update();
    image.addEventListener('load', update);
    const resizeObserver = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(update);
    resizeObserver?.observe(target);
    return () => {
      image.removeEventListener('load', update);
      resizeObserver?.disconnect();
    };
  }, [kind, source]);

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
    const position = getPreviewScrollPosition(scrollKey);
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
  const imageStyle = {
    ...(imageDisplaySize ? { width: `${imageDisplaySize.width}px`, height: `${imageDisplaySize.height}px` } : {}),
    transform: `translate3d(${imagePan.x}px, ${imagePan.y}px, 0) scale(${imageZoom / 100})`,
    transformOrigin: 'center center',
  };
  const viewOriginal = async () => {
    if (!entry || !metadata || kind !== 'image' || !source || imageVariant === 'original' || originalLoading) return;
    const controller = new AbortController();
    imageRequestControllerRef.current?.abort();
    imageRequestControllerRef.current = controller;
    setOriginalLoading(true);
    setError('');
    try {
      const result = await readContent(instanceId, entry.relativePath, root.revision, { variant: 'original', signal: controller.signal });
      const nextSource = URL.createObjectURL(new Blob([result.data], { type: metadata.mimeType }));
      if (imageRequestControllerRef.current !== controller) {
        URL.revokeObjectURL(nextSource);
        return;
      }
      setImageZoom(100);
      setImagePan({ x: 0, y: 0 });
      setImageDragging(false);
      imageDragRef.current = null;
      setImageVariant('original');
      setSource(nextSource);
    } catch (reason) {
      if (controller.signal.aborted) return;
      const originalError = (reason instanceof Error ? reason : new Error('Unable to load original image')) as FileSystemError;
      if (originalError.code === 'filesystem_root_changed' && originalError.root) onRootChanged();
      setError(originalError.message);
      onToast('Unable to load original image.', 'error');
    } finally {
      if (imageRequestControllerRef.current === controller) {
        imageRequestControllerRef.current = null;
        setOriginalLoading(false);
      }
    }
  };
  const download = async () => {
    if (!entry || !metadata) return;
    try {
      const result = await readContent(instanceId, entry.relativePath, root.revision, { download: true });
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
          <button autoFocus className="icon-button file-preview-back-terminal" type="button" onClick={onBackToTerminal} title="Back to Terminal" aria-label="Back to Terminal" data-testid="file-preview-back-terminal"><SquareTerminal size={17} aria-hidden="true" /></button>
          {kind === 'markdown' && <button className="icon-button" type="button" onClick={() => setMarkdownSource((value) => !value)} title={markdownSource ? 'Rendered markdown' : 'Markdown source'} aria-label={markdownSource ? 'Rendered markdown' : 'Markdown source'}><Search size={16} aria-hidden="true" /></button>}
          {kind === 'image' && source && <label className="filesystem-image-zoom" title="Image zoom"><ZoomOut size={14} aria-hidden="true" /><input id="file-preview-image-zoom" data-testid="file-preview-image-zoom" aria-label="Image zoom" type="range" min={MIN_IMAGE_ZOOM} max={MAX_IMAGE_ZOOM} step={IMAGE_ZOOM_STEP} value={imageZoom} onChange={handleImageZoomChange} /><output htmlFor="file-preview-image-zoom">{imageZoom}%</output><ZoomIn size={14} aria-hidden="true" /></label>}
          {kind === 'image' && source && <button className={`icon-button ${imageVariant === 'original' ? 'selected' : ''}`} type="button" onClick={() => void viewOriginal()} disabled={imageVariant === 'original' || originalLoading} title={imageVariant === 'original' ? 'Original loaded' : 'View original'} aria-label={imageVariant === 'original' ? 'Original loaded' : 'View original'} data-testid="file-preview-view-original">{originalLoading ? <LoaderCircle size={16} className="spin" aria-hidden="true" /> : <ScanSearch size={16} aria-hidden="true" />}</button>}
          <button className="icon-button" type="button" onClick={() => void download()} title="Download" aria-label="Download"><Download size={16} aria-hidden="true" /></button>
        </div>
      </header>
      <div ref={previewBodyRef} className={`filesystem-preview-body ${kind === 'image' ? 'filesystem-preview-body-image' : ''}`}>
        {!hasPreviewContent && loading && <div className="filesystem-loading"><LoaderCircle size={18} className="spin" aria-hidden="true" /> Loading preview</div>}
        {!hasPreviewContent && !loading && error && <div className="filesystem-error">{error}</div>}
        {hasPreviewContent && kind === 'image' && source && <img ref={imageRef} className={`filesystem-image-viewer${imageDragging ? ' dragging' : ''}`} src={source} alt={entry.name} draggable={false} style={imageStyle} onLoad={updateImageDisplaySize} onPointerDown={handleImagePointerDown} onPointerMove={handleImagePointerMove} onPointerUp={finishImageDrag} onPointerCancel={finishImageDrag} onLostPointerCapture={finishImageDrag} />}
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
