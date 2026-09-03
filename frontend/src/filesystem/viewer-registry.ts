import type { FileMetadata } from './filesystem-types';

export type ViewerKind = 'text' | 'markdown' | 'image' | 'video' | 'pdf' | 'raw';

const imageTypes = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml', 'image/avif']);
const videoTypes = new Set(['video/mp4', 'video/webm', 'video/ogg', 'video/quicktime']);

export type ViewerDescriptor = {
  id: string;
  label: string;
  kind: ViewerKind;
  priority: number;
  probe: (metadata: FileMetadata) => boolean;
  extensionProbe?: (name: string) => boolean;
};

const builtInViewers: ViewerDescriptor[] = [
  { id: 'pdf', label: 'PDF', kind: 'pdf', priority: 100, probe: (metadata) => metadata.mimeType === 'application/pdf', extensionProbe: (name) => name.toLowerCase().endsWith('.pdf') },
  { id: 'image', label: 'Image', kind: 'image', priority: 90, probe: (metadata) => imageTypes.has(metadata.mimeType) },
  { id: 'video', label: 'Video', kind: 'video', priority: 90, probe: (metadata) => videoTypes.has(metadata.mimeType) },
  { id: 'markdown', label: 'Markdown', kind: 'markdown', priority: 80, probe: (metadata) => metadata.mimeType === 'text/markdown', extensionProbe: (name) => /\.(md|markdown)$/i.test(name) },
  { id: 'text', label: 'Text', kind: 'text', priority: 20, probe: (metadata) => metadata.mimeType.startsWith('text/') || metadata.mimeType === 'application/json' || metadata.mimeType === 'application/xml', extensionProbe: (name) => /\.(txt|log|json|yaml|yml|toml|go|ts|tsx|js|jsx|css|html|sh|py|rs|java|sql)$/i.test(name) },
];

const extensions: ViewerDescriptor[] = [];

export function registerViewer(descriptor: ViewerDescriptor): () => void {
  if (!descriptor.id || !descriptor.kind || !Number.isFinite(descriptor.priority)) throw new Error('Invalid viewer descriptor');
  extensions.push(descriptor);
  return () => {
    const index = extensions.indexOf(descriptor);
    if (index >= 0) extensions.splice(index, 1);
  };
}

export function viewerDescriptors(): ViewerDescriptor[] {
  return [...builtInViewers, ...extensions].sort((left, right) => right.priority - left.priority);
}

export function viewerFor(metadata: FileMetadata): ViewerKind {
  const descriptors = viewerDescriptors();
  for (const descriptor of descriptors) {
    if (!descriptor.extensionProbe) continue;
    try {
      if (descriptor.extensionProbe(metadata.name)) return descriptor.kind;
    } catch {
      // A viewer probe is optional; one broken extension must not block fallback.
    }
  }
  for (const descriptor of descriptors) {
    try {
      if (descriptor.probe(metadata)) return descriptor.kind;
    } catch {
      // A viewer probe is optional; one broken extension must not block fallback.
    }
  }
  return 'raw';
}

export function viewerLabel(kind: ViewerKind): string {
  return viewerDescriptors().find((descriptor) => descriptor.kind === kind)?.label || (kind === 'raw' ? 'Binary' : kind);
}
