import { Download, FileQuestion, LoaderCircle, ScanSearch, Search, SquareTerminal, ZoomIn, ZoomOut } from 'lucide-react';
import type { FileEntry, RootContext } from './filesystem-types';
import { formatSize } from './file-preview-helpers';
import { useFilePreviewController } from './file-preview-controller';
import { IMAGE_ZOOM_STEP, MAX_IMAGE_ZOOM, MIN_IMAGE_ZOOM } from './file-preview-image';
import { MarkdownPreview } from './markdown-preview';
import { viewerLabel } from './viewer-registry';

export { fitImageSize } from './file-preview-image';

type Props = {
  instanceId: string;
  root: RootContext;
  entry: FileEntry | null;
  onBackToTerminal: () => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  onRootChanged: () => void;
};

export function FilePreview({ instanceId, root, entry, onBackToTerminal, onToast, onRootChanged }: Props) {
  const {
    error, finishImageDrag, handleImagePointerDown, handleImagePointerMove, handleImageZoomChange,
    hasPreviewContent, imageDragging, imageRef, imageStyle, imageVariant, imageZoom, kind, loading,
    markdownSource, metadata, originalLoading, previewBodyRef, source, text, toggleMarkdownSource,
    updateImageDisplaySize, viewOriginal, download,
  } = useFilePreviewController({ instanceId, root, entry, onToast, onRootChanged });

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
          {kind === 'markdown' && <button className="icon-button" type="button" onClick={toggleMarkdownSource} title={markdownSource ? 'Rendered markdown' : 'Markdown source'} aria-label={markdownSource ? 'Rendered markdown' : 'Markdown source'}><Search size={16} aria-hidden="true" /></button>}
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
