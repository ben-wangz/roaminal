import type { FileMetadata } from './filesystem-types';

export type ViewerKind = 'text' | 'markdown' | 'image' | 'video' | 'pdf' | 'raw';

const imageTypes = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml', 'image/avif']);
const videoTypes = new Set(['video/mp4', 'video/webm', 'video/ogg', 'video/quicktime']);

export function viewerFor(metadata: FileMetadata): ViewerKind {
  const lower = metadata.name.toLowerCase();
  if (metadata.mimeType === 'application/pdf' || lower.endsWith('.pdf')) return 'pdf';
  if (imageTypes.has(metadata.mimeType)) return 'image';
  if (videoTypes.has(metadata.mimeType)) return 'video';
  if (metadata.mimeType === 'text/markdown' || lower.endsWith('.md') || lower.endsWith('.markdown')) return 'markdown';
  if (metadata.mimeType.startsWith('text/') || metadata.mimeType === 'application/json' || metadata.mimeType === 'application/xml' || /\.(txt|log|json|yaml|yml|toml|go|ts|tsx|js|jsx|css|html|sh|py|rs|java|sql)$/i.test(lower)) return 'text';
  return 'raw';
}

export function viewerLabel(kind: ViewerKind): string {
  return ({ text: 'Text', markdown: 'Markdown', image: 'Image', video: 'Video', pdf: 'PDF', raw: 'Binary' })[kind];
}
