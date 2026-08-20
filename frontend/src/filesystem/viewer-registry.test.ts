import { describe, expect, it } from 'vitest';
import { registerViewer, viewerFor } from './viewer-registry';
import type { FileMetadata } from './filesystem-types';

function metadata(name: string, mimeType: string): FileMetadata {
  return {
    name,
    relativePath: name,
    absolutePath: `/workspace/${name}`,
    type: 'file',
    size: 10,
    modifiedAt: null,
    mode: 0,
    symlink: false,
    mimeType,
    encoding: 'utf-8',
    capabilities: { read: true, range: true, stream: true, download: true },
    consistencyToken: 'token',
  };
}

describe('filesystem viewer registry', () => {
  it('prefers trusted MIME metadata for media and documents', () => {
    expect(viewerFor(metadata('asset.bin', 'image/png'))).toBe('image');
    expect(viewerFor(metadata('asset.bin', 'video/mp4'))).toBe('video');
    expect(viewerFor(metadata('asset.bin', 'application/pdf'))).toBe('pdf');
  });

  it('uses markdown and safe text fallbacks by name and MIME', () => {
    expect(viewerFor(metadata('README.md', 'text/plain'))).toBe('markdown');
    expect(viewerFor(metadata('notes', 'text/plain'))).toBe('text');
    expect(viewerFor(metadata('archive.bin', 'application/octet-stream'))).toBe('raw');
  });

  it('allows an extension descriptor to override the built-in priority', () => {
    const unregister = registerViewer({
      id: 'fixture',
      label: 'Fixture',
      kind: 'text',
      priority: 200,
      probe: (value) => value.name === 'fixture.bin',
    });
    expect(viewerFor(metadata('fixture.bin', 'application/octet-stream'))).toBe('text');
    unregister();
  });

  it('continues to the safe fallback when an extension probe throws', () => {
    const unregister = registerViewer({ id: 'broken', label: 'Broken', kind: 'text', priority: 200, probe: () => { throw new Error('probe failed'); } });
    expect(viewerFor(metadata('archive.bin', 'application/octet-stream'))).toBe('raw');
    unregister();
  });
});
