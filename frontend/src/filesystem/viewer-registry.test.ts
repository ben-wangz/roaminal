import { describe, expect, it } from 'vitest';
import { viewerFor } from './viewer-registry';
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
});
