import { describe, expect, it, vi } from 'vitest';
import {
  exitFullscreenDocument,
  fullscreenCapability,
  fullscreenControlState,
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
  it('reports an unsupported capability when there is no target', () => {
    expect(fullscreenCapability(null, documentLike({ fullscreenEnabled: true }))).toEqual({
      supported: false,
      standard: false,
      prefixed: false,
    });
  });

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

  it('does not accept a prefixed method when the document denies fullscreen', () => {
    const webkitRequestFullscreen = vi.fn();
    const targetElement = target({ webkitRequestFullscreen });

    expect(fullscreenCapability(targetElement, documentLike({ fullscreenEnabled: false }))).toEqual({
      supported: false,
      standard: false,
      prefixed: false,
    });
  });

  it('supports the legacy WebKit request and exit spellings', () => {
    const webkitRequestFullScreen = vi.fn();
    const webkitCancelFullScreen = vi.fn();
    const targetElement = target({ webkitRequestFullScreen });
    const doc = documentLike({ webkitCancelFullScreen });

    expect(fullscreenCapability(targetElement, doc)).toEqual({ supported: true, standard: false, prefixed: true });
    requestFullscreenForTarget(targetElement, fullscreenCapability(targetElement, doc));
    exitFullscreenDocument(doc);
    expect(webkitRequestFullScreen).toHaveBeenCalledOnce();
    expect(webkitCancelFullScreen).toHaveBeenCalledOnce();
  });

  it('maps lifecycle inputs to stable control states', () => {
    expect(fullscreenControlState(false, false, false)).toBe('unsupported');
    expect(fullscreenControlState(false, true, false)).toBe('available');
    expect(fullscreenControlState(false, true, true)).toBe('pending');
    expect(fullscreenControlState(true, true, false)).toBe('active');
    expect(fullscreenControlState(true, false, false)).toBe('active');
  });

  it('uses the active standard or prefixed element for state', () => {
    const standard = target({});
    const prefixed = target({});
    expect(fullscreenElement(documentLike({ fullscreenElement: standard }))).toBe(standard);
    expect(fullscreenElement(documentLike({ webkitFullscreenElement: prefixed }))).toBe(prefixed);
    expect(fullscreenElement(documentLike({ webkitCurrentFullScreenElement: prefixed }))).toBe(prefixed);
  });

  it('keeps request and exit calls direct while preserving resolved promises', async () => {
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

  it('preserves request and exit Promise rejections for the hook to report', async () => {
    const requestError = new Error('request rejected');
    const exitError = new Error('exit rejected');
    const requestFullscreen = vi.fn(() => Promise.reject(requestError));
    const exitFullscreen = vi.fn(() => Promise.reject(exitError));
    const targetElement = target({ requestFullscreen });
    const doc = documentLike({ fullscreenEnabled: true, exitFullscreen });

    await expect(Promise.resolve(requestFullscreenForTarget(targetElement, fullscreenCapability(targetElement, doc)))).rejects.toBe(requestError);
    await expect(Promise.resolve(exitFullscreenDocument(doc))).rejects.toBe(exitError);
  });

  it('surfaces a throwing target method to the caller instead of treating it as supported success', () => {
    const requestFullscreen = vi.fn(() => { throw new Error('rejected'); });
    const targetElement = target({ requestFullscreen });
    const capability = fullscreenCapability(targetElement, documentLike({ fullscreenEnabled: true }));

    expect(() => requestFullscreenForTarget(targetElement, capability)).toThrow('rejected');
  });
});
