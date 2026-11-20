import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { userEvent } from 'vitest/browser';
import '../i18n/config';
import ProfilePictureUploadDialog from './ProfilePictureUploadDialog';

// An existing committed repo image, reused instead of adding a binary fixture.
// It is square (512x512), which is what the dialog's aspect={1} cropper expects.
// Referenced by its served public path: importing from public/ makes Vite warn
// on every run ("assets in the public directory are served at the root path").
const AVATAR_URL = '/android-chrome-512x512.png';

// Real-browser tier (issue #2539). ProfilePictureUploadDialog.test.tsx (jsdom)
// documents its crop path as an explicit gap: the crop library depends on real
// image measurement / ResizeObserver machinery jsdom does not drive, so
// onCropComplete never fires there. Here a real <img> loads and the cropper
// measures it, so the crop area has a real, non-zero box.
//
// frontend/AGENTS.md trap 1: explicit cleanup, no globals.
afterEach(cleanup);

async function avatarFile(): Promise<File> {
  const response = await fetch(AVATAR_URL);
  // A 404 would otherwise surface as a confusing "cropper never rendered".
  expect(response.ok).toBe(true);
  const blob = await response.blob();
  return new File([blob], 'avatar.png', { type: 'image/png' });
}

test('a loaded image gives the cropper a real, image-shaped crop area', async () => {
  render(<ProfilePictureUploadDialog open onClose={() => {}} onUpload={async () => {}} />);

  // userEvent.upload drives the real file input; the dialog converts it to a
  // data URL and swaps to the crop view.
  await userEvent.upload(screen.getByLabelText('Select an image'), await avatarFile());

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

  // The cropper only renders once the browser reports natural dimensions.
  const image = document.querySelector<HTMLImageElement>('.reactEasyCrop_Image');
  expect(image).not.toBeNull();
  await waitFor(() => expect(image?.naturalWidth).toBe(512));
  expect(image?.naturalHeight).toBe(512);

  // The rendered image has a real box (the measurement jsdom cannot do)...
  const rendered = (image as HTMLImageElement).getBoundingClientRect();
  expect(rendered.width).toBeGreaterThan(0);
  expect(rendered.height).toBeGreaterThan(0);

  // ...and the crop area is non-zero and matches the image's aspect ratio.
  await waitFor(() => expect(cropArea.getBoundingClientRect().width).toBeGreaterThan(0));
  const crop = cropArea.getBoundingClientRect();
  const imageAspect =
    (image as HTMLImageElement).naturalWidth / (image as HTMLImageElement).naturalHeight;
  expect(crop.height).toBeGreaterThan(0);
  expect(crop.width / crop.height).toBeCloseTo(imageAspect, 2);
});
