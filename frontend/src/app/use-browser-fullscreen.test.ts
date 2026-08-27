import { describe, expect, it, vi } from 'vitest';
import {
  exitFullscreenDocument,
  fullscreenCapability,
  fullscreenElement,
  requestFullscreenForTarget,
  type FullscreenTarget,
} from './use-browser-fullscreen';

function target(value: Partial<FullscreenTarget>): FullscreenTarget {
  return value as FullscreenTarget;
}

function documentLike(value: Record<string, unknown>): Document {
  return value as unknown as Document;
}

describe('browser fullscreen adapter', () => {
  it('requires both the standard request method and document permission', () => {
    const requestFullscreen = vi.fn();
    const targetElement = target({ requestFullscreen });

    expect(fullscreenCapability(targetElement, documentLike({ fullscreenEnabled: false }))).toEqual({
      supported: false,
      standard: false,
      prefixed: false,
    });
    expect(fullscreenCapability(targetElement, documentLike({ fullscreenEnabled: true }))).toEqual({
      supported: true,
      standard: true,
      prefixed: false,
    });
  });

  it('accepts a generic prefixed WebKit target when the document exposes it', () => {
    const webkitRequestFullscreen = vi.fn();
    const targetElement = target({ webkitRequestFullscreen });
    const doc = documentLike({ webkitFullscreenEnabled: true });

    expect(fullscreenCapability(targetElement, doc)).toEqual({ supported: true, standard: false, prefixed: true });
    requestFullscreenForTarget(targetElement, fullscreenCapability(targetElement, doc));
    expect(webkitRequestFullscreen).toHaveBeenCalledOnce();
  });

  it('uses the active standard or prefixed element for state', () => {
    const standard = target({});
    const prefixed = target({});
    expect(fullscreenElement(documentLike({ fullscreenElement: standard }))).toBe(standard);
    expect(fullscreenElement(documentLike({ webkitFullscreenElement: prefixed }))).toBe(prefixed);
  });

  it('keeps request and exit calls direct while preserving rejected promises', async () => {
    const requestFullscreen = vi.fn(() => Promise.resolve());
    const exitFullscreen = vi.fn(() => Promise.resolve());
    const targetElement = target({ requestFullscreen });
    const doc = documentLike({ fullscreenEnabled: true, exitFullscreen });
    const capability = fullscreenCapability(targetElement, doc);

    await expect(Promise.resolve(requestFullscreenForTarget(targetElement, capability))).resolves.toBeUndefined();
    await expect(Promise.resolve(exitFullscreenDocument(doc))).resolves.toBeUndefined();
    expect(requestFullscreen).toHaveBeenCalledOnce();
    expect(exitFullscreen).toHaveBeenCalledOnce();
  });

  it('surfaces a throwing target method to the caller instead of treating it as supported success', () => {
    const requestFullscreen = vi.fn(() => { throw new Error('rejected'); });
    const targetElement = target({ requestFullscreen });
    const capability = fullscreenCapability(targetElement, documentLike({ fullscreenEnabled: true }));

    expect(() => requestFullscreenForTarget(targetElement, capability)).toThrow('rejected');
  });
});
