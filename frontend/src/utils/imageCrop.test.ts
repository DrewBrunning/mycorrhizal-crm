import { describe, expect, it } from 'vitest';
import { cropToControls, MIN_CROP_ZOOM, squareCrop } from './imageCrop';

describe('squareCrop', () => {
  it('zoom 1 is the largest square, centred along the long edge', () => {
    // 200x100 landscape: a 100x100 square is w=0.5, h=1.
    const crop = squareCrop(200, 100, 1, 0.5, 0.5);
    expect(crop).toEqual({ x: 0.25, y: 0, w: 0.5, h: 1 });
  });

  it('keeps the rectangle inside the image at the extremes', () => {
    const topLeft = squareCrop(200, 100, 0.5, 0, 0);
    expect(topLeft).toEqual({ x: 0, y: 0, w: 0.25, h: 0.5 });
    const bottomRight = squareCrop(200, 100, 0.5, 1, 1);
    expect(bottomRight.x + bottomRight.w).toBeCloseTo(1);
    expect(bottomRight.y + bottomRight.h).toBeCloseTo(1);
  });

  it('clamps out-of-range input', () => {
    const crop = squareCrop(100, 100, 5, -3, 9);
    expect(crop).toEqual({ x: 0, y: 0, w: 1, h: 1 });
    expect(squareCrop(100, 100, 0, 0, 0).w).toBeCloseTo(MIN_CROP_ZOOM);
  });

  it('falls back to the whole image for unknown dimensions', () => {
    expect(squareCrop(0, 0, 0.5, 0.5, 0.5)).toEqual({ x: 0, y: 0, w: 1, h: 1 });
  });
});

describe('cropToControls', () => {
  it('round-trips a crop produced by squareCrop', () => {
    const crop = squareCrop(300, 200, 0.6, 0.25, 0.75);
    const controls = cropToControls(300, 200, crop);
    expect(controls.zoom).toBeCloseTo(0.6);
    expect(controls.posX).toBeCloseTo(0.25);
    expect(controls.posY).toBeCloseTo(0.75);
  });

  it('opens centred and fully zoomed out when the crop spans the whole image', () => {
    expect(cropToControls(300, 200, { x: 0, y: 0, w: 1, h: 1 })).toEqual({
      zoom: 1,
      posX: 0.5,
      posY: 0.5,
    });
  });

  it('opens at defaults when no crop is stored', () => {
    expect(cropToControls(300, 200, null)).toEqual({ zoom: 1, posX: 0.5, posY: 0.5 });
  });
});
