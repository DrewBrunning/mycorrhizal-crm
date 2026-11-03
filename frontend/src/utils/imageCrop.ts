import type { ImageCrop } from '../api/contactImages';

// Smallest zoom the crop editor allows: the square side as a fraction of the
// shorter edge. Mirrors Android's MIN_CROP_ZOOM (ImageCropMath.kt).
export const MIN_CROP_ZOOM = 0.1;

export interface CropControls {
  zoom: number;
  posX: number;
  posY: number;
}

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

// A square crop (the avatar is square-cropped server-side) as fractions of the
// full variant (ADR 0040 decision 10). `zoom` is the square side relative to the
// shorter edge (1 = the largest square); `posX`/`posY` in 0..1 slide it across
// the leftover space, so the rectangle always stays inside the image.
export function squareCrop(
  width: number,
  height: number,
  zoom: number,
  posX: number,
  posY: number,
): ImageCrop {
  if (width <= 0 || height <= 0) return { x: 0, y: 0, w: 1, h: 1 };
  const side = clamp(zoom, MIN_CROP_ZOOM, 1) * Math.min(width, height);
  const w = side / width;
  const h = side / height;
  return {
    x: clamp(posX, 0, 1) * (1 - w),
    y: clamp(posY, 0, 1) * (1 - h),
    w,
    h,
  };
}

// Inverse of squareCrop, so the editor opens at the stored crop. A crop that
// leaves no room to slide on an axis opens centred on it.
export function cropToControls(
  width: number,
  height: number,
  crop: ImageCrop | null | undefined,
): CropControls {
  if (!crop || width <= 0 || height <= 0) return { zoom: 1, posX: 0.5, posY: 0.5 };
  const zoom = clamp((crop.w * width) / Math.min(width, height), MIN_CROP_ZOOM, 1);
  const spareX = 1 - crop.w;
  const spareY = 1 - crop.h;
  return {
    zoom,
    posX: spareX > 1e-9 ? clamp(crop.x / spareX, 0, 1) : 0.5,
    posY: spareY > 1e-9 ? clamp(crop.y / spareY, 0, 1) : 0.5,
  };
}
