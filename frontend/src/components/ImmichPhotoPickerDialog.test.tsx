import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ImmichPerson } from '../api/immich';
import ImmichPhotoPickerDialog from './ImmichPhotoPickerDialog';

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
      // Fallback: respond with a placeholder blob for any unhandled URL,
      // so AuthImg's thumbnail/image fetches always succeed in tests.
      if (url.includes('/thumbnail') || url.includes('/image')) {
        const blob = new Blob(['ok'], { type: 'image/png' });
        return { ok: true, blob: async () => blob };
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

const people: ImmichPerson[] = [{ id: 'p-alice', name: 'Alice Example' }];

function renderPicker(
  overrides: Partial<React.ComponentProps<typeof ImmichPhotoPickerDialog>> = {},
) {
  const defaults: React.ComponentProps<typeof ImmichPhotoPickerDialog> = {
    open: true,
    onClose: vi.fn(),
    contactUid: 'contact-1',
    isLinked: true,
    onFetchPeople: vi.fn().mockResolvedValue(people),
    onLinkPerson: vi.fn().mockResolvedValue(undefined),
    onImageSelected: vi.fn(),
    ...overrides,
  };
  return render(<ImmichPhotoPickerDialog {...defaults} />);
}

test('an unlinked contact sees the person search dialog first', async () => {
  mockFetchByUrl({});
  renderPicker({ isLinked: false });

  await waitFor(() => {
    expect(screen.getByText('Link an Immich person')).toBeInTheDocument();
  });
  // The browse-photos grid is not shown until a person is linked.
  expect(screen.queryByText('Choose a photo from Immich')).not.toBeInTheDocument();
});

test('linking a person transitions in place to browsing their photos', async () => {
  mockFetchByUrl({
    '/immich/contacts/contact-1/assets': () => ({
      assets: [{ id: 'asset-1', occurred_at: '2026-08-03T10:00:00Z' }],
    }),
  });
  const onLinkPerson = vi.fn().mockResolvedValue(undefined);
  renderPicker({ isLinked: false, onLinkPerson });

  await waitFor(() => expect(screen.getByText('Alice Example')).toBeInTheDocument());
  fireEvent.click(screen.getByText('Alice Example'));

  await waitFor(() => expect(onLinkPerson).toHaveBeenCalledWith(people[0]));
  await waitFor(() => {
    expect(screen.getByText('Choose a photo from Immich')).toBeInTheDocument();
  });
});

test('an already-linked contact browses photos directly', async () => {
  mockFetchByUrl({
    '/immich/contacts/contact-1/assets': () => ({
      assets: [{ id: 'asset-1', occurred_at: '2026-08-03T10:00:00Z' }],
    }),
  });
  renderPicker();

  await waitFor(() => {
    expect(screen.getByText('Choose a photo from Immich')).toBeInTheDocument();
  });
  await screen.findByAltText('Photo from Immich');
  await screen.findByAltText('Current Immich thumbnail');
});

test('a load failure surfaces the real error message', async () => {
  mockFetchByUrl({
    '/immich/contacts/contact-1/assets': () => {
      throw new Error('unreachable');
    },
  });
  renderPicker();

  await waitFor(() => {
    expect(screen.getByText('unreachable')).toBeInTheDocument();
  });
});

test('picking a photo fetches it and hands back a data URL', async () => {
  const imageBlob = new Blob(['fake-image-bytes'], { type: 'image/jpeg' });
  mockFetchByUrl({
    '/immich/contacts/contact-1/assets': () => ({ assets: [] }),
    '/thumbnail': () => imageBlob,
  });
  const onImageSelected = vi.fn();
  renderPicker({ onImageSelected });

  const thumb = await screen.findByAltText('Current Immich thumbnail');
  fireEvent.click(thumb.closest('button')!);

  await waitFor(() => expect(onImageSelected).toHaveBeenCalled());
  expect(onImageSelected.mock.calls[0][0]).toMatch(/^data:/);
});
