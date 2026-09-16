import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ImmichPerson } from '../api/immich';
import ProfilePictureUploadDialog from './ProfilePictureUploadDialog';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mockFetchByUrl(handlers: Record<string, (init?: RequestInit) => unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, _init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          const result = respond(_init);
          const blob = new Blob(['ok'], { type: 'image/png' });
          return {
            ok: true,
            json: async () => (result instanceof Error ? Promise.reject(result) : result),
            blob: async () => (result instanceof Blob ? result : blob),
          };
        }
      }
      if (url.includes('/thumbnail') || url.includes('/image')) {
        const blob = new Blob(['ok'], { type: 'image/png' });
        return { ok: true, blob: async () => blob };
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

function renderDialog(
  overrides: Partial<React.ComponentProps<typeof ProfilePictureUploadDialog>> = {},
) {
  const defaults: React.ComponentProps<typeof ProfilePictureUploadDialog> = {
    open: true,
    onClose: vi.fn(),
    onUpload: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return render(<ProfilePictureUploadDialog {...defaults} />);
}

// handleFileSelect's validation branches. Skips the canvas-drawing
// getCroppedImg path (out of scope) -- these only assert what happens
// before/around actually cropping.
function selectFile(file: File) {
  fireEvent.change(screen.getByLabelText('Select an image'), { target: { files: [file] } });
}

test('handleFileSelect rejects a non-image file type', () => {
  renderDialog();

  selectFile(new File(['hello'], 'notes.txt', { type: 'text/plain' }));

  expect(screen.getByText('Please select an image file')).toBeInTheDocument();
  // Stays on the file-selection view -- no crop UI mounted.
  expect(screen.queryByLabelText('Zoom')).not.toBeInTheDocument();
});

test('handleFileSelect rejects an oversized image file', () => {
  renderDialog();

  const oversized = new File(['x'], 'huge.png', { type: 'image/png' });
  Object.defineProperty(oversized, 'size', { value: 11 * 1024 * 1024 });
  selectFile(oversized);

  expect(screen.getByText('File is too large. Maximum size is 10MB.')).toBeInTheDocument();
  expect(screen.queryByLabelText('Zoom')).not.toBeInTheDocument();
});

test('handleFileSelect accepts a valid image and advances to the crop step', async () => {
  renderDialog();

  selectFile(new File(['fake-image-bytes'], 'photo.png', { type: 'image/png' }));

  await waitFor(() => expect(screen.getByLabelText('Zoom')).toBeInTheDocument());
  expect(screen.queryByText('Please select an image file')).not.toBeInTheDocument();
});

// --- handleFetchFromUrl's error paths ---------------------------------------

test('handleFetchFromUrl surfaces a non-ok response as an error', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: false,
      json: async () => ({ error: 'remote server refused' }),
    })),
  );
  renderDialog();

  fireEvent.change(screen.getByPlaceholderText('Paste image URL...'), {
    target: { value: 'https://example.com/photo.jpg' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Fetch' }));

  await waitFor(() => expect(screen.getByText('remote server refused')).toBeInTheDocument());
  expect(screen.queryByLabelText('Zoom')).not.toBeInTheDocument();
});

test('handleFetchFromUrl falls back to a generic message when the error response has none', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: false,
      json: async () => ({}),
    })),
  );
  renderDialog();

  fireEvent.change(screen.getByPlaceholderText('Paste image URL...'), {
    target: { value: 'https://example.com/photo.jpg' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Fetch' }));

  await waitFor(() =>
    expect(screen.getByText('Failed to fetch image from URL')).toBeInTheDocument(),
  );
});

test('handleFetchFromUrl rejects a non-image content-type', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      headers: new Headers({ 'content-type': 'text/html' }),
      blob: async () => new Blob(['<html></html>'], { type: 'text/html' }),
    })),
  );
  renderDialog();

  fireEvent.change(screen.getByPlaceholderText('Paste image URL...'), {
    target: { value: 'https://example.com/not-an-image' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Fetch' }));

  await waitFor(() => expect(screen.getByText('Please select an image file')).toBeInTheDocument());
});

test('handleFetchFromUrl rejects an oversized blob', async () => {
  const oversizedBlob = new Blob(['x'], { type: 'image/png' });
  Object.defineProperty(oversizedBlob, 'size', { value: 11 * 1024 * 1024 });
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      headers: new Headers({ 'content-type': 'image/png' }),
      blob: async () => oversizedBlob,
    })),
  );
  renderDialog();

  fireEvent.change(screen.getByPlaceholderText('Paste image URL...'), {
    target: { value: 'https://example.com/huge.png' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Fetch' }));

  await waitFor(() =>
    expect(screen.getByText('File is too large. Maximum size is 10MB.')).toBeInTheDocument(),
  );
});

test('handleFetchFromUrl succeeds for a valid, correctly-typed image', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      headers: new Headers({ 'content-type': 'image/png' }),
      blob: async () => new Blob(['fake-image-bytes'], { type: 'image/png' }),
    })),
  );
  renderDialog();

  fireEvent.change(screen.getByPlaceholderText('Paste image URL...'), {
    target: { value: 'https://example.com/photo.png' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Fetch' }));

  await waitFor(() => expect(screen.getByLabelText('Zoom')).toBeInTheDocument());
});

test('the Immich entry point is hidden when Immich is not configured for this contact', () => {
  renderDialog();
  expect(screen.queryByText('Choose from Immich')).not.toBeInTheDocument();
});

test('the Immich entry point opens the photo picker when configured', async () => {
  mockFetchByUrl({});
  const people: ImmichPerson[] = [{ id: 'p-1', name: 'Alice' }];
  renderDialog({
    immich: {
      contactUid: 'contact-1',
      isLinked: false,
      onFetchPeople: vi.fn().mockResolvedValue(people),
      onLinkPerson: vi.fn().mockResolvedValue(undefined),
    },
  });

  fireEvent.click(screen.getByText('Choose from Immich'));

  // Not linked yet — the photo picker delegates to the person search dialog.
  await waitFor(() => {
    expect(screen.getByText('Link an Immich person')).toBeInTheDocument();
  });
});

test('picking a photo from Immich flows into the existing crop step', async () => {
  const imageBlob = new Blob(['fake-image-bytes'], { type: 'image/jpeg' });
  mockFetchByUrl({
    '/immich/contacts/contact-1/assets': () => ({ assets: [] }),
    '/thumbnail': () => imageBlob,
  });

  renderDialog({
    immich: {
      contactUid: 'contact-1',
      isLinked: true,
      onFetchPeople: vi.fn(),
      onLinkPerson: vi.fn(),
    },
  });

  fireEvent.click(screen.getByText('Choose from Immich'));

  await waitFor(() => expect(screen.getByText('Choose a photo from Immich')).toBeInTheDocument());
  await screen.findByAltText('Current Immich thumbnail');
  const thumb = screen.getByAltText('Current Immich thumbnail');
  fireEvent.click(thumb.closest('button')!);

  await waitFor(() => {
    expect(screen.getByLabelText('Zoom')).toBeInTheDocument();
  });
});
