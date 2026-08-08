import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ProfilePictureUploadDialog from './ProfilePictureUploadDialog';
import { ImmichPerson } from '../api/immich';

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
    })
  );
}

function renderDialog(overrides: Partial<React.ComponentProps<typeof ProfilePictureUploadDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ProfilePictureUploadDialog> = {
    open: true,
    onClose: vi.fn(),
    onUpload: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return render(<ProfilePictureUploadDialog {...defaults} />);
}

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
