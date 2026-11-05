import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../api/client';
import type { ContactPhoto } from '../api/contactImages';
import ContactImageGallery from './ContactImageGallery';

const getContactPhotos = vi.fn();
const deleteContactImage = vi.fn();
const uploadContactImage = vi.fn();
const setContactMainImage = vi.fn();
const clearContactMainImage = vi.fn();
const patchContactImage = vi.fn();
const adoptContactImage = vi.fn();

vi.mock('../api/contactImages', async () => {
  const actual =
    await vi.importActual<typeof import('../api/contactImages')>('../api/contactImages');
  return {
    ...actual,
    getContactPhotos: (...args: unknown[]) => getContactPhotos(...args),
    deleteContactImage: (...args: unknown[]) => deleteContactImage(...args),
    uploadContactImage: (...args: unknown[]) => uploadContactImage(...args),
    setContactMainImage: (...args: unknown[]) => setContactMainImage(...args),
    clearContactMainImage: (...args: unknown[]) => clearContactMainImage(...args),
    patchContactImage: (...args: unknown[]) => patchContactImage(...args),
    adoptContactImage: (...args: unknown[]) => adoptContactImage(...args),
  };
});

const own: ContactPhoto = {
  id: 9,
  source: { kind: 'contact', id: 4, label: 'Contact' },
  mime_type: 'image/jpeg',
  width: 200,
  height: 100,
  size_bytes: 1200,
  sensitivity: 'normal',
  taken_at: null,
  taken_date: null,
  created_at: '2026-01-01T00:00:00Z',
  file_url: '/api/v1/contacts/4/images/9/file',
  thumbnail_url: '/api/v1/contacts/4/images/9/thumbnail',
  crop: null,
};

// Ids are only unique per owner table: this note image shares id 9 with `own`.
const noteImage: ContactPhoto = {
  ...own,
  source: { kind: 'note', id: 3, label: 'Note' },
  file_url: '/api/v1/notes/3/images/9/file',
  thumbnail_url: '/api/v1/notes/3/images/9/thumbnail',
};

const activityImage: ContactPhoto = {
  ...own,
  id: 5,
  source: { kind: 'activity', id: 8, label: 'Lunch' },
  file_url: '/api/v1/activities/8/images/5/file',
  thumbnail_url: '/api/v1/activities/8/images/5/thumbnail',
};

function listOf(...photos: ContactPhoto[]) {
  return { photos, total: photos.length };
}

async function openMenu(tile: string) {
  const el = await screen.findByTestId(tile);
  fireEvent.click(within(el).getByRole('button', { name: 'Photo actions' }));
}

describe('ContactImageGallery (Photos tab)', () => {
  beforeEach(() => {
    getContactPhotos.mockReset().mockResolvedValue(listOf(own));
    deleteContactImage.mockReset().mockResolvedValue(undefined);
    uploadContactImage.mockReset().mockResolvedValue({ image: own, deduplicated: false });
    setContactMainImage
      .mockReset()
      .mockResolvedValue({ message: 'ok', main_image_id: 9, revision: 8 });
    clearContactMainImage.mockReset().mockResolvedValue({ message: 'ok', revision: 9 });
    patchContactImage.mockReset().mockResolvedValue({ ...own });
    adoptContactImage.mockReset().mockResolvedValue({ image: own, deduplicated: false });
  });

  afterEach(cleanup);

  it('renders thumbnails from the aggregated photos endpoint', async () => {
    render(<ContactImageGallery contactId={4} />);
    const thumb = await screen.findByRole('img', { name: /contact photo/i });
    expect(getContactPhotos).toHaveBeenCalledWith(4);
    expect(thumb.getAttribute('src')).toMatch(/\/api\/v1\/contacts\/4\/images\/9\/thumbnail$/);
  });

  it('labels note and activity photos by source and leaves own photos unlabelled', async () => {
    getContactPhotos.mockResolvedValue(listOf(own, noteImage, activityImage));
    render(<ContactImageGallery contactId={4} />);
    expect(await screen.findByText('Note: Note')).toBeTruthy();
    expect(screen.getByText('Activity: Lunch')).toBeTruthy();
    const ownTile = screen.getByTestId('contact-photo-contact-9');
    expect(within(ownTile).queryByText(/Note|Activity/)).toBeNull();
  });

  it('badges private and secret photos', async () => {
    getContactPhotos.mockResolvedValue(
      listOf({ ...own, sensitivity: 'private' }, { ...activityImage, sensitivity: 'secret' }),
    );
    render(<ContactImageGallery contactId={4} />);
    expect(await screen.findByText('Private')).toBeTruthy();
    expect(screen.getByText('Secret')).toBeTruthy();
  });

  it('marks only the own photo matching the main image id, not a note image with the same id', async () => {
    getContactPhotos.mockResolvedValue(listOf(own, noteImage));
    render(<ContactImageGallery contactId={4} mainImageId={9} />);
    await screen.findByTestId('contact-photo-contact-9');
    expect(
      within(screen.getByTestId('contact-photo-contact-9')).getByLabelText('Main photo'),
    ).toBeTruthy();
    expect(
      within(screen.getByTestId('contact-photo-note-9')).queryByLabelText('Main photo'),
    ).toBeNull();
  });

  it('sets the main photo with the contact revision and refreshes the contact', async () => {
    const onContactChanged = vi.fn();
    render(
      <ContactImageGallery
        contactId={4}
        editable
        revision={7}
        onContactChanged={onContactChanged}
      />,
    );
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Set as main photo' }));
    await waitFor(() => expect(setContactMainImage).toHaveBeenCalledWith(4, 9, 7));
    await waitFor(() => expect(onContactChanged).toHaveBeenCalled());
  });

  it('reports a stale revision (412), refetches the contact and does not claim success', async () => {
    setContactMainImage.mockRejectedValue(new ApiError('stale', 'PRECONDITION_FAILED', 412));
    const onContactChanged = vi.fn();
    render(
      <ContactImageGallery
        contactId={4}
        editable
        revision={1}
        onContactChanged={onContactChanged}
      />,
    );
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Set as main photo' }));
    expect(await screen.findByText(/changed elsewhere/i)).toBeTruthy();
    expect(onContactChanged).toHaveBeenCalled();
  });

  it('shows a generic error when setting the main photo fails otherwise', async () => {
    setContactMainImage.mockRejectedValue(new Error('boom'));
    render(<ContactImageGallery contactId={4} editable revision={1} />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Set as main photo' }));
    expect(await screen.findByText('Could not set the main photo.')).toBeTruthy();
  });

  it('clears the main photo', async () => {
    const onContactChanged = vi.fn();
    render(
      <ContactImageGallery
        contactId={4}
        editable
        mainImageId={9}
        revision={8}
        onContactChanged={onContactChanged}
      />,
    );
    await openMenu('contact-photo-contact-9');
    expect(screen.queryByRole('menuitem', { name: 'Set as main photo' })).toBeNull();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Clear main photo' }));
    await waitFor(() => expect(clearContactMainImage).toHaveBeenCalledWith(4, 8));
    await waitFor(() => expect(onContactChanged).toHaveBeenCalled());
  });

  it('does not offer a private or secret photo as the main image', async () => {
    getContactPhotos.mockResolvedValue(listOf({ ...own, sensitivity: 'secret' }));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-contact-9');
    expect(screen.queryByRole('menuitem', { name: 'Set as main photo' })).toBeNull();
    expect(screen.getByRole('menuitem', { name: 'Delete photo' })).toBeTruthy();
  });

  it('offers only "add to contact photos" for note and activity photos', async () => {
    getContactPhotos.mockResolvedValue(listOf(noteImage));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-note-9');
    expect(screen.getByRole('menuitem', { name: 'Add to contact photos' })).toBeTruthy();
    expect(screen.queryByRole('menuitem', { name: 'Delete photo' })).toBeNull();
    expect(screen.queryByRole('menuitem', { name: 'Set as main photo' })).toBeNull();
    expect(screen.queryByRole('menuitem', { name: /crop/i })).toBeNull();
  });

  it('adopts a note photo, reloads and tells the user', async () => {
    getContactPhotos.mockResolvedValue(listOf(noteImage));
    const onChanged = vi.fn();
    render(<ContactImageGallery contactId={4} editable onChanged={onChanged} />);
    await openMenu('contact-photo-note-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Add to contact photos' }));
    await waitFor(() =>
      expect(adoptContactImage).toHaveBeenCalledWith(4, {
        source_kind: 'note',
        source_id: 3,
        image_id: 9,
      }),
    );
    await waitFor(() => expect(getContactPhotos).toHaveBeenCalledTimes(2));
    expect(await screen.findByText('Added to the contact photos.')).toBeTruthy();
    expect(onChanged).toHaveBeenCalled();
  });

  it('says so when the adopted photo already existed', async () => {
    adoptContactImage.mockResolvedValue({ image: own, deduplicated: true });
    getContactPhotos.mockResolvedValue(listOf(activityImage));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-activity-5');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Add to contact photos' }));
    expect(await screen.findByText('That photo is already in the contact photos.')).toBeTruthy();
  });

  it('shows an error when adopting fails', async () => {
    adoptContactImage.mockRejectedValue(new Error('full'));
    getContactPhotos.mockResolvedValue(listOf(noteImage));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-note-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Add to contact photos' }));
    expect(await screen.findByText('Could not add the photo to the contact photos.')).toBeTruthy();
  });

  it('opens the crop editor at the stored crop and saves a new rectangle', async () => {
    // 200x100: zoom 0.5 -> side 50px -> w 0.25, h 0.5; posX 1 -> x 0.75; posY 0 -> y 0.
    getContactPhotos.mockResolvedValue(
      listOf({ ...own, crop: { x: 0.75, y: 0, w: 0.25, h: 0.5 } }),
    );
    const onContactChanged = vi.fn();
    render(<ContactImageGallery contactId={4} editable onContactChanged={onContactChanged} />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Crop avatar' }));
    const dialog = await screen.findByRole('dialog');
    const zoom = within(dialog).getByRole('slider', { name: 'Zoom' }) as HTMLInputElement;
    expect(Number(zoom.value)).toBeCloseTo(0.5);
    expect(
      Number(
        (within(dialog).getByRole('slider', { name: 'Horizontal position' }) as HTMLInputElement)
          .value,
      ),
    ).toBeCloseTo(1);

    fireEvent.click(within(dialog).getByRole('button', { name: 'Save crop' }));
    await waitFor(() => expect(patchContactImage).toHaveBeenCalledTimes(1));
    const [contactId, imageId, patch] = patchContactImage.mock.calls[0];
    expect([contactId, imageId]).toEqual([4, 9]);
    expect(patch.crop.x).toBeCloseTo(0.75);
    expect(patch.crop.w).toBeCloseTo(0.25);
    expect(patch.crop.h).toBeCloseTo(0.5);
    await waitFor(() => expect(onContactChanged).toHaveBeenCalled());
  });

  it('opens at the whole photo with no stored crop, and a changed zoom saves a smaller square', async () => {
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Crop avatar' }));
    const dialog = await screen.findByRole('dialog');
    const zoom = within(dialog).getByRole('slider', { name: 'Zoom' });
    expect((zoom as HTMLInputElement).value).toBe('1');
    fireEvent.change(zoom, { target: { value: '0.5' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save crop' }));
    await waitFor(() => expect(patchContactImage).toHaveBeenCalled());
    expect(patchContactImage.mock.calls[0][2].crop.w).toBeCloseTo(0.25);
  });

  // v1.6.0 review: the gallery showed a photo's sensitivity but had no way to change it.
  it('marks an own photo private from its menu and refreshes the contact', async () => {
    patchContactImage.mockResolvedValue({ ...own, sensitivity: 'private' });
    const onContactChanged = vi.fn();
    render(<ContactImageGallery contactId={4} editable onContactChanged={onContactChanged} />);
    await openMenu('contact-photo-contact-9');
    expect(screen.queryByRole('menuitem', { name: 'Mark as normal' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Mark as private' }));
    await waitFor(() =>
      expect(patchContactImage).toHaveBeenCalledWith(4, 9, { sensitivity: 'private' }),
    );
    expect(await screen.findByText('Private')).toBeInTheDocument();
    // A non-normal main image clears the avatar server-side, bumping the revision.
    expect(onContactChanged).toHaveBeenCalled();
  });

  it('shows an error when the sensitivity change fails', async () => {
    patchContactImage.mockRejectedValue(new Error('patch failed'));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Mark as secret' }));
    expect(await screen.findByText("Could not change the photo's sensitivity.")).toBeTruthy();
  });

  it('offers no sensitivity change for a note photo shown in the gallery', async () => {
    getContactPhotos.mockResolvedValue(listOf(noteImage));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-note-9');
    expect(screen.queryByRole('menuitem', { name: 'Mark as private' })).not.toBeInTheDocument();
  });

  it('clears the crop with an explicit null', async () => {
    getContactPhotos.mockResolvedValue(listOf({ ...own, crop: { x: 0, y: 0, w: 0.5, h: 1 } }));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Crop avatar' }));
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Use whole photo' }),
    );
    await waitFor(() => expect(patchContactImage).toHaveBeenCalledWith(4, 9, { crop: null }));
  });

  it('shows an error when saving the crop fails', async () => {
    patchContactImage.mockRejectedValue(new Error('bad crop'));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Crop avatar' }));
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Save crop' }),
    );
    expect(await screen.findByText('Could not save the crop.')).toBeTruthy();
  });

  it('deletes an own photo from the menu and refreshes the contact', async () => {
    const onContactChanged = vi.fn();
    render(<ContactImageGallery contactId={4} editable onContactChanged={onContactChanged} />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Delete photo' }));
    await waitFor(() => expect(deleteContactImage).toHaveBeenCalledWith(4, 9));
    await waitFor(() => expect(onContactChanged).toHaveBeenCalled());
  });

  it('renders no actions on the read-only surface', async () => {
    render(<ContactImageGallery contactId={4} />);
    await screen.findByRole('img', { name: /contact photo/i });
    expect(screen.queryByRole('button', { name: 'Photo actions' })).toBeNull();
  });

  it('renders nothing read-only when there are no photos', async () => {
    getContactPhotos.mockResolvedValue(listOf());
    const { container } = render(<ContactImageGallery contactId={4} />);
    await waitFor(() => expect(getContactPhotos).toHaveBeenCalledWith(4));
    expect(container).toBeEmptyDOMElement();
  });

  it('shows a load error even on a read-only gallery', async () => {
    getContactPhotos.mockRejectedValue(new Error('down'));
    render(<ContactImageGallery contactId={4} />);
    expect(await screen.findByText('Could not load photos.')).toBeTruthy();
  });

  it('uploads selected files, reloads and notifies the parent', async () => {
    const onChanged = vi.fn();
    const { container } = render(
      <ContactImageGallery contactId={4} editable onChanged={onChanged} />,
    );
    await screen.findByRole('img', { name: /contact photo/i });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' });
    fireEvent.change(input, { target: { files: [file] } });
    await waitFor(() => expect(uploadContactImage).toHaveBeenCalledWith(4, file));
    await waitFor(() => expect(getContactPhotos).toHaveBeenCalledTimes(2));
    expect(onChanged).toHaveBeenCalled();
  });

  it('says so when an upload matched a photo already in the gallery', async () => {
    uploadContactImage.mockResolvedValue({ image: own, deduplicated: true });
    const { container } = render(<ContactImageGallery contactId={4} editable />);
    await screen.findByRole('img', { name: /contact photo/i });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, {
      target: { files: [new File(['x'], 'photo.jpg', { type: 'image/jpeg' })] },
    });
    expect(
      await screen.findByText(
        'That photo is already in the contact photos, so it was not added again.',
      ),
    ).toBeTruthy();
  });

  it('shows an upload error', async () => {
    uploadContactImage.mockRejectedValue(new Error('upload failed'));
    const { container } = render(<ContactImageGallery contactId={4} editable />);
    await screen.findByRole('img', { name: /contact photo/i });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, {
      target: { files: [new File(['x'], 'photo.jpg', { type: 'image/jpeg' })] },
    });
    expect(await screen.findByText('Could not upload the photo.')).toBeTruthy();
  });

  it('shows a delete error', async () => {
    deleteContactImage.mockRejectedValue(new Error('delete failed'));
    render(<ContactImageGallery contactId={4} editable />);
    await openMenu('contact-photo-contact-9');
    fireEvent.click(screen.getByRole('menuitem', { name: 'Delete photo' }));
    expect(await screen.findByText('Could not delete the photo.')).toBeTruthy();
  });

  it('opens the lightbox and shows the full image', async () => {
    render(<ContactImageGallery contactId={4} />);
    fireEvent.click(await screen.findByRole('img', { name: /contact photo/i }));
    expect(screen.getByRole('dialog')).toBeTruthy();
  });

  it('offers an add button that opens the file picker on an editable empty gallery', async () => {
    getContactPhotos.mockResolvedValue(listOf());
    const { container } = render(<ContactImageGallery contactId={4} editable />);
    const button = await screen.findByRole('button', { name: 'Add photos' });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const click = vi.spyOn(input, 'click').mockImplementation(() => {});
    fireEvent.click(button);
    expect(click).toHaveBeenCalled();
  });
});
