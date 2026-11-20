import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { userEvent } from 'vitest/browser';
import '../i18n/config';
import ProfilePictureUploadDialog from './ProfilePictureUploadDialog';

// Real-browser tier (issue #2539). ProfilePictureUploadDialog.test.tsx (jsdom)
// documents its crop path as an explicit gap: the crop library depends on real
// image measurement / ResizeObserver machinery jsdom does not drive, so
// onCropComplete never fires there. Here a real <img> loads and the cropper
// measures it, so the crop area has a real, non-zero box.
//
// frontend/AGENTS.md trap 1: explicit cleanup, no globals.
afterEach(cleanup);

// Generated in the browser rather than committed as a binary fixture. 3:2 is
// deliberately NOT the dialog's crop aspect: a square fixture makes "the crop
// matches the image aspect" true by construction, so it cannot tell a working
// cropper from one that ignores `aspect` or swaps axes.
async function wideImageFile(): Promise<File> {
  const canvas = document.createElement('canvas');
  canvas.width = 600;
  canvas.height = 400;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('2d canvas context unavailable');
  ctx.fillStyle = '#3366cc';
  ctx.fillRect(0, 0, 600, 400);
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(40, 40, 220, 140);
  ctx.fillStyle = '#ffcc00';
  ctx.fillRect(360, 210, 180, 120);
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (produced) => (produced ? resolve(produced) : reject(new Error('canvas.toBlob was empty'))),
      'image/png',
    );
  });
  return new File([blob], 'wide.png', { type: 'image/png' });
}

test('a 3:2 image is measured, then cropped to the dialog’s square aspect', async () => {
  render(<ProfilePictureUploadDialog open onClose={() => {}} onUpload={async () => {}} />);

  // userEvent.upload drives the real file input; the dialog converts it to a
  // data URL and swaps to the crop view.
  await userEvent.upload(screen.getByLabelText('Select an image'), await wideImageFile());

  // AppDialog renders into a portal, so query the document, not render()'s
  // container element.
  const cropArea = (await waitFor(
    () => {
      const el = document.querySelector<HTMLElement>('.reactEasyCrop_CropArea');
      expect(el).not.toBeNull();
      return el as HTMLElement;
    },
    { timeout: 8000 },
  )) as HTMLElement;

  // The cropper only renders once the browser reports natural dimensions;
  // 600x400 proves it measured the real decoded image, non-trivially.
  const image = document.querySelector<HTMLImageElement>('.reactEasyCrop_Image');
  expect(image).not.toBeNull();
  await waitFor(() => expect(image?.naturalWidth).toBe(600));
  expect(image?.naturalHeight).toBe(400);

  // The rendered image has a real box (the measurement jsdom cannot do), and
  // it was scaled into the 350px-tall container rather than drawn at 600x400.
  const rendered = (image as HTMLImageElement).getBoundingClientRect();
  expect(rendered.width).toBeGreaterThan(0);
  expect(rendered.height).toBeGreaterThan(0);
  expect(rendered.width).toBeLessThan(600);
  expect(rendered.height).toBeLessThan(400);

  await waitFor(() => expect(cropArea.getBoundingClientRect().width).toBeGreaterThan(0));
  const crop = cropArea.getBoundingClientRect();
  expect(crop.height).toBeGreaterThan(0);

  // Square because the DIALOG configures aspect={1}
  // (ProfilePictureUploadDialog.tsx), not because the image is square: this
  // fixture is 3:2. A cropper that used the image's aspect, or swapped axes,
  // produces a 1.5 (or 0.67) ratio here and fails.
  expect(crop.width / crop.height).toBeCloseTo(1, 2);

  // The 3:2 image is wider than the square crop box: react-easy-crop fitted
  // the image past the crop horizontally instead of shrinking the crop to the
  // image's own shape.
  expect(rendered.width).toBeGreaterThan(crop.width);
});
