export type ImageDisplaySize = {
  width: number;
  height: number;
  scale: number;
};

export type ImagePan = { x: number; y: number };
export type ImageDrag = ImagePan & { pointerId: number; startX: number; startY: number };

export const MIN_IMAGE_ZOOM = 25;
export const MAX_IMAGE_ZOOM = 400;
export const IMAGE_ZOOM_STEP = 25;

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
