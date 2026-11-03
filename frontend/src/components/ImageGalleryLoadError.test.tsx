import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ActivityImageGallery from './ActivityImageGallery';
import ContactImageGallery from './ContactImageGallery';
import NoteImageGallery from './NoteImageGallery';

// Characterization of the gallery load-error state across the three owners
// (#2312). A failed load must be visible on a read-only gallery for every owner;
// a read-only gallery with no images and no error renders nothing.
const getActivityImages = vi.fn();
const getNoteImages = vi.fn();
const getContactPhotos = vi.fn();

vi.mock('../api/activities', async () => {
  const actual = await vi.importActual<typeof import('../api/activities')>('../api/activities');
  return { ...actual, getActivityImages: (...a: unknown[]) => getActivityImages(...a) };
});
vi.mock('../api/notes', async () => {
  const actual = await vi.importActual<typeof import('../api/notes')>('../api/notes');
  return { ...actual, getNoteImages: (...a: unknown[]) => getNoteImages(...a) };
});
vi.mock('../api/contactImages', async () => {
  const actual =
    await vi.importActual<typeof import('../api/contactImages')>('../api/contactImages');
  return { ...actual, getContactPhotos: (...a: unknown[]) => getContactPhotos(...a) };
});

const owners = [
  {
    name: 'activity',
    mock: getActivityImages,
    empty: { images: [], total: 0 },
    ui: () => <ActivityImageGallery activityId={4} />,
  },
  {
    name: 'note',
    mock: getNoteImages,
    empty: { images: [], total: 0 },
    ui: () => <NoteImageGallery noteId={4} />,
  },
  {
    name: 'contact',
    mock: getContactPhotos,
    empty: { photos: [], total: 0 },
    ui: () => <ContactImageGallery contactId={4} />,
  },
];

describe('image gallery load error (all owners)', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  for (const owner of owners) {
    it(`${owner.name}: shows a load failure on a read-only gallery`, async () => {
      owner.mock.mockReset().mockRejectedValue(new Error('down'));
      render(owner.ui());
      expect(await screen.findByText('Could not load photos.')).toBeTruthy();
    });

    it(`${owner.name}: renders nothing read-only when empty without an error`, async () => {
      owner.mock.mockReset().mockResolvedValue(owner.empty);
      const { container } = render(owner.ui());
      await waitFor(() => expect(owner.mock).toHaveBeenCalled());
      await waitFor(() => expect(container).toBeEmptyDOMElement());
    });
  }
});
