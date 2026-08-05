import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ImmichPhotoPickerDialog from './ImmichPhotoPickerDialog';
import { ImmichPerson } from '../api/immich';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mockFetchByUrl(handlers: Record<string, (init?: RequestInit) => unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          const result = respond(init);
          if (result instanceof Blob) {
            return { ok: true, blob: async () => result };
          }
          return { ok: true, json: async () => result };
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
}

const people: ImmichPerson[] = [{ id: 'p-alice', name: 'Alice Example' }];

function renderPicker(overrides: Partial<React.ComponentProps<typeof ImmichPhotoPickerDialog>> = {}) {
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
  expect(screen.getAllByAltText('Photo from Immich')).toHaveLength(1);
  expect(screen.getByAltText('Current Immich thumbnail')).toBeInTheDocument();
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

  await waitFor(() => expect(screen.getByAltText('Current Immich thumbnail')).toBeInTheDocument());
  fireEvent.click(screen.getByAltText('Current Immich thumbnail').closest('button')!);

  await waitFor(() => expect(onImageSelected).toHaveBeenCalled());
  expect(onImageSelected.mock.calls[0][0]).toMatch(/^data:/);
});
