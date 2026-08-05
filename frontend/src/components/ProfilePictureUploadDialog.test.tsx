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
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          return { ok: true, json: async () => respond(init) };
        }
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
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/assets')) return { ok: true, json: async () => ({ assets: [] }) };
      if (url.includes('/thumbnail')) return { ok: true, blob: async () => imageBlob };
      throw new Error(`unexpected fetch: ${url}`);
    })
  );

  renderDialog({
    immich: {
      contactUid: 'contact-1',
      isLinked: true,
      onFetchPeople: vi.fn(),
      onLinkPerson: vi.fn(),
    },
  });

  fireEvent.click(screen.getByText('Choose from Immich'));

  await waitFor(() => expect(screen.getByAltText('Current Immich thumbnail')).toBeInTheDocument());
  fireEvent.click(screen.getByAltText('Current Immich thumbnail').closest('button')!);

  // The picker's data URL becomes imageSrc — the crop UI (zoom slider) appears.
  await waitFor(() => {
    expect(screen.getByLabelText('Zoom')).toBeInTheDocument();
  });
});
