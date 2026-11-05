import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ImageGallery, {
  activityImageGalleryApi,
  contactImageGalleryApi,
  type ImageGalleryPrefix,
  noteImageGalleryApi,
} from './ImageGallery';

// One gallery serves three owners (issue #2312). This suite pins each owner's
// behaviour (which API function it calls, its URL shape, its i18n strings) and
// the shared behaviour, including that a load error is shown read-only too.

const getActivityImages = vi.fn();
const deleteActivityImage = vi.fn();
const uploadActivityImage = vi.fn();
const patchActivityImage = vi.fn();
const getNoteImages = vi.fn();
const deleteNoteImage = vi.fn();
const uploadNoteImage = vi.fn();
const patchNoteImage = vi.fn();
const getContactImages = vi.fn();
const deleteContactImage = vi.fn();
const uploadContactImage = vi.fn();
const patchContactImage = vi.fn();

vi.mock('../api/activities', async () => {
  const actual = await vi.importActual<typeof import('../api/activities')>('../api/activities');
  return {
    ...actual,
    getActivityImages: (...args: unknown[]) => getActivityImages(...args),
    deleteActivityImage: (...args: unknown[]) => deleteActivityImage(...args),
    uploadActivityImage: (...args: unknown[]) => uploadActivityImage(...args),
    patchActivityImage: (...args: unknown[]) => patchActivityImage(...args),
  };
});
vi.mock('../api/notes', async () => {
  const actual = await vi.importActual<typeof import('../api/notes')>('../api/notes');
  return {
    ...actual,
    getNoteImages: (...args: unknown[]) => getNoteImages(...args),
    deleteNoteImage: (...args: unknown[]) => deleteNoteImage(...args),
    uploadNoteImage: (...args: unknown[]) => uploadNoteImage(...args),
    patchNoteImage: (...args: unknown[]) => patchNoteImage(...args),
  };
});
vi.mock('../api/contactImages', async () => {
  const actual =
    await vi.importActual<typeof import('../api/contactImages')>('../api/contactImages');
  return {
    ...actual,
    getContactImages: (...args: unknown[]) => getContactImages(...args),
    deleteContactImage: (...args: unknown[]) => deleteContactImage(...args),
    uploadContactImage: (...args: unknown[]) => uploadContactImage(...args),
    patchContactImage: (...args: unknown[]) => patchContactImage(...args),
  };
});

interface OwnerCase {
  name: string;
  i18nPrefix: ImageGalleryPrefix;
  urlSegment: string;
  altPattern: RegExp;
  list: ReturnType<typeof vi.fn>;
  remove: ReturnType<typeof vi.fn>;
  upload: ReturnType<typeof vi.fn>;
  patch: ReturnType<typeof vi.fn>;
}

const owners: OwnerCase[] = [
  {
    name: 'activity',
    i18nPrefix: 'activityImages',
    urlSegment: 'activities',
    altPattern: /activity photo/i,
    list: getActivityImages,
    remove: deleteActivityImage,
    upload: uploadActivityImage,
    patch: patchActivityImage,
  },
  {
    name: 'note',
    i18nPrefix: 'noteImages',
    urlSegment: 'notes',
    altPattern: /note photo/i,
    list: getNoteImages,
    remove: deleteNoteImage,
    upload: uploadNoteImage,
    patch: patchNoteImage,
  },
  {
    name: 'contact',
    i18nPrefix: 'contactImages',
    urlSegment: 'contacts',
    altPattern: /contact photo/i,
    list: getContactImages,
    remove: deleteContactImage,
    upload: uploadContactImage,
    patch: patchContactImage,
  },
];

function imageFor(owner: OwnerCase) {
  return {
    id: 9,
    uuid: '0f0e0d0c-0b0a-4908-8706-050403020100',
    mime_type: 'image/jpeg' as const,
    width: 100,
    height: 80,
    size_bytes: 1200,
    sensitivity: 'normal' as const,
    taken_at: null,
    taken_date: null,
    created_at: '2026-01-01T00:00:00Z',
    file_url: `/api/v1/${owner.urlSegment}/4/images/9/file`,
    thumbnail_url: `/api/v1/${owner.urlSegment}/4/images/9/thumbnail`,
  };
}

// The real owner props, as each call site passes them.
function renderGallery(
  owner: OwnerCase,
  props: { editable?: boolean; onChanged?: () => void } = {},
) {
  const common = { ownerId: 4, i18nPrefix: owner.i18nPrefix, ...props };
  switch (owner.name) {
    case 'activity':
      return render(<ImageGallery {...common} api={activityImageGalleryApi} />);
    case 'note':
      return render(<ImageGallery {...common} api={noteImageGalleryApi} />);
    default:
      return render(<ImageGallery {...common} api={contactImageGalleryApi} />);
  }
}

describe.each(owners)('ImageGallery ($name)', (owner) => {
  const image = imageFor(owner);

  beforeEach(() => {
    for (const o of owners) {
      o.list.mockReset().mockResolvedValue({ images: [imageFor(o)], total: 1 });
      o.remove.mockReset().mockResolvedValue(undefined);
      o.upload.mockReset().mockResolvedValue(imageFor(o));
      o.patch.mockReset().mockResolvedValue({ ...imageFor(o), sensitivity: 'private' });
    }
  });

  afterEach(cleanup);

  it('renders thumbnails from the owner-nested endpoint', async () => {
    renderGallery(owner);
    const thumb = await screen.findByRole('img', { name: owner.altPattern });
    const src = thumb.getAttribute('src') ?? '';
    expect(new URL(src, window.location.href).pathname).toBe(image.thumbnail_url);
  });

  it('deletes a photo from the editable surface', async () => {
    renderGallery(owner, { editable: true });
    fireEvent.click(await screen.findByRole('button', { name: /delete photo/i }));
    await waitFor(() => expect(owner.remove).toHaveBeenCalledWith(4, 9));
  });

  it('renders nothing read-only when there are no images', async () => {
    owner.list.mockResolvedValue({ images: [], total: 0 });
    const { container } = renderGallery(owner);
    await waitFor(() => expect(owner.list).toHaveBeenCalledWith(4));
    expect(container).toBeEmptyDOMElement();
  });

  it('shows a loading spinner while the images are fetched', () => {
    owner.list.mockReturnValue(new Promise(() => {}));
    renderGallery(owner);
    expect(screen.getByLabelText(/loading photos/i)).toBeInTheDocument();
  });

  it('shows an error when loading images fails', async () => {
    owner.list.mockRejectedValue(new Error('load failed'));
    renderGallery(owner, { editable: true });
    expect(await screen.findByText('Could not load photos.')).toBeInTheDocument();
  });

  // Issue #2312: the activity gallery used to hide a failed load when
  // read-only while notes and contacts showed it. Show it everywhere.
  it('shows a load error on the read-only surface instead of hiding it', async () => {
    owner.list.mockRejectedValue(new Error('load failed'));
    renderGallery(owner);
    expect(await screen.findByText('Could not load photos.')).toBeInTheDocument();
  });

  it('uploads every selected photo then reloads and notifies the parent', async () => {
    const onChanged = vi.fn();
    const first = new File(['a'], 'a.jpg', { type: 'image/jpeg' });
    const second = new File(['b'], 'b.png', { type: 'image/png' });
    renderGallery(owner, { editable: true, onChanged });

    await screen.findByRole('img', { name: owner.altPattern });
    const input = screen.getByLabelText(/add photos/i);
    const inputClick = vi.fn();
    input.addEventListener('click', inputClick);
    fireEvent.click(screen.getByRole('button', { name: /add photos/i }));
    expect(inputClick).toHaveBeenCalledTimes(1);
    fireEvent.change(input, { target: { files: [first, second] } });

    await waitFor(() => {
      expect(owner.upload).toHaveBeenCalledWith(4, first);
      expect(owner.upload).toHaveBeenCalledWith(4, second);
      expect(owner.list).toHaveBeenCalledTimes(2);
      expect(onChanged).toHaveBeenCalledTimes(1);
    });
  });

  it('shows an error when an upload fails', async () => {
    owner.upload.mockRejectedValue(new Error('upload failed'));
    renderGallery(owner, { editable: true });

    const input = await screen.findByLabelText(/add photos/i);
    fireEvent.change(input, {
      target: { files: [new File(['a'], 'a.jpg', { type: 'image/jpeg' })] },
    });

    expect(await screen.findByText('Could not upload the photo.')).toBeInTheDocument();
  });

  it('shows an error when deleting fails', async () => {
    owner.remove.mockRejectedValue(new Error('delete failed'));
    renderGallery(owner, { editable: true });
    fireEvent.click(await screen.findByRole('button', { name: /delete photo/i }));
    expect(await screen.findByText('Could not delete the photo.')).toBeInTheDocument();
  });

  it('opens the full image in a lightbox and closes it', async () => {
    renderGallery(owner);
    fireEvent.click(await screen.findByRole('img', { name: owner.altPattern }));

    const dialog = await screen.findByRole('dialog');
    const full = within(dialog).getByRole('img', { name: owner.altPattern });
    const src = full.getAttribute('src') ?? '';
    expect(new URL(src, window.location.href).pathname).toBe(image.file_url);

    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  // v1.6.0 review: a note or activity photo's sensitivity could be read but
  // not changed, and nothing showed it was private.
  it('labels a private or secret photo, read-only too', async () => {
    owner.list.mockResolvedValue({ images: [{ ...image, sensitivity: 'secret' }], total: 1 });
    renderGallery(owner);
    expect(await screen.findByText('Secret')).toBeInTheDocument();
  });

  it('changes a photo sensitivity from the viewer on the editable surface', async () => {
    renderGallery(owner, { editable: true });
    fireEvent.click(await screen.findByRole('img', { name: owner.altPattern }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Private' }));
    await waitFor(() => expect(owner.patch).toHaveBeenCalledWith(4, 9, { sensitivity: 'private' }));
    await waitFor(() =>
      expect(within(dialog).getByRole('button', { name: 'Private' })).toHaveAttribute(
        'aria-pressed',
        'true',
      ),
    );
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByText('Private')).toBeInTheDocument();
  });

  it('shows an error when the sensitivity change fails', async () => {
    owner.patch.mockRejectedValue(new Error('patch failed'));
    renderGallery(owner, { editable: true });
    fireEvent.click(await screen.findByRole('img', { name: owner.altPattern }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Secret' }));
    expect(
      await within(dialog).findByText("Could not change the photo's sensitivity."),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Normal' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('ignores a click on the already-selected sensitivity', async () => {
    renderGallery(owner, { editable: true });
    fireEvent.click(await screen.findByRole('img', { name: owner.altPattern }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Normal' }));
    expect(owner.patch).not.toHaveBeenCalled();
  });

  it('updates only the changed photo and notifies the parent', async () => {
    const onChanged = vi.fn();
    const other = { ...image, id: 10, sensitivity: 'private' as const };
    owner.list.mockResolvedValue({ images: [image, other], total: 2 });
    renderGallery(owner, { editable: true, onChanged });
    const thumbs = await screen.findAllByRole('img', { name: owner.altPattern });
    fireEvent.click(thumbs[0]);
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Secret' }));
    await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1));
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    // The untouched photo keeps its own label.
    expect(screen.getAllByText('Private')).toHaveLength(2);
  });

  it('offers no sensitivity control on the read-only surface', async () => {
    renderGallery(owner);
    fireEvent.click(await screen.findByRole('img', { name: owner.altPattern }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByRole('button', { name: 'Private' })).not.toBeInTheDocument();
  });

  it('has no delete control on the read-only surface', async () => {
    renderGallery(owner);
    await screen.findByRole('img', { name: owner.altPattern });
    expect(screen.queryByRole('button', { name: /delete photo/i })).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/add photos/i)).not.toBeInTheDocument();
  });
});
