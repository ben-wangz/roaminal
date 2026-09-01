import { describe, expect, it } from 'vitest';
import { fitImageSize } from './file-preview';

describe('fitImageSize', () => {
  it('fits a large image inside the available preview area without changing its ratio', () => {
    expect(fitImageSize(4000, 2000, 1000, 700)).toEqual({ width: 1000, height: 500, scale: 0.25 });
    expect(fitImageSize(2000, 4000, 1000, 700)).toEqual({ width: 350, height: 700, scale: 0.175 });
  });

  it('keeps a smaller image at its natural size', () => {
    expect(fitImageSize(640, 480, 1000, 700)).toEqual({ width: 640, height: 480, scale: 1 });
  });

  it('rejects incomplete dimensions', () => {
    expect(fitImageSize(0, 480, 1000, 700)).toBeNull();
    expect(fitImageSize(640, 480, 0, 700)).toBeNull();
  });
});
