type ScrollPosition = {
  top: number;
  left: number;
};

const previewScrollPositions = new Map<string, ScrollPosition>();
const MAX_PREVIEW_SCROLL_POSITIONS = 100;

export function savePreviewScrollPosition(key: string | null, target: HTMLElement | null): void {
  if (!key || !target) return;
  previewScrollPositions.delete(key);
  previewScrollPositions.set(key, { top: target.scrollTop, left: target.scrollLeft });
  while (previewScrollPositions.size > MAX_PREVIEW_SCROLL_POSITIONS) {
    const oldest = previewScrollPositions.keys().next().value;
    if (oldest === undefined) break;
    previewScrollPositions.delete(oldest);
  }
}

export function getPreviewScrollPosition(key: string): ScrollPosition | undefined {
  return previewScrollPositions.get(key);
}

export function formatSize(size: number | null): string {
  if (size === null) return 'size unavailable';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
