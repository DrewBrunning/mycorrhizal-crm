import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ActivityImageGallery from './ActivityImageGallery';

const getActivityImages = vi.fn();
const deleteActivityImage = vi.fn();
const uploadActivityImage = vi.fn();

vi.mock('../api/activities', async () => {
  const actual = await vi.importActual<typeof import('../api/activities')>('../api/activities');
  return {
    ...actual,
    getActivityImages: (...args: unknown[]) => getActivityImages(...args),
    deleteActivityImage: (...args: unknown[]) => deleteActivityImage(...args),
    uploadActivityImage: (...args: unknown[]) => uploadActivityImage(...args),
  };
});

const image = {
  id: 9,
  uuid: '0f0e0d0c-0b0a-4908-8706-050403020100',
  activity_id: 4,
  mime_type: 'image/jpeg' as const,
  width: 100,
  height: 80,
  size_bytes: 1200,
  sensitivity: 'normal' as const,
  taken_at: null,
  taken_date: null,
  created_at: '2026-01-01T00:00:00Z',
  file_url: '/api/v1/activities/4/images/9/file',
  thumbnail_url: '/api/v1/activities/4/images/9/thumbnail',
};

describe('ActivityImageGallery', () => {
  beforeEach(() => {
    getActivityImages.mockReset().mockResolvedValue({ images: [image], total: 1 });
    deleteActivityImage.mockReset().mockResolvedValue(undefined);
    uploadActivityImage.mockReset().mockResolvedValue(image);
  });

  afterEach(cleanup);

  it('renders thumbnails from the owner-nested endpoint', async () => {
    render(<ActivityImageGallery activityId={4} />);
    const thumb = await screen.findByRole('img', { name: /activity photo/i });
    const src = thumb.getAttribute('src') ?? '';
    expect(new URL(src, window.location.href).pathname).toBe(
      '/api/v1/activities/4/images/9/thumbnail',
    );
  });

  it('deletes a photo from the editable surface', async () => {
    render(<ActivityImageGallery activityId={4} editable />);
    fireEvent.click(await screen.findByRole('button', { name: /delete photo/i }));
    await waitFor(() => expect(deleteActivityImage).toHaveBeenCalledWith(4, 9));
  });

  it('renders nothing read-only when there are no images', async () => {
    getActivityImages.mockResolvedValue({ images: [], total: 0 });
    const { container } = render(<ActivityImageGallery activityId={4} />);
    await waitFor(() => expect(getActivityImages).toHaveBeenCalledWith(4));
    expect(container).toBeEmptyDOMElement();
  });

  it('shows a loading spinner while the images are fetched', () => {
    getActivityImages.mockReturnValue(new Promise(() => {}));
    render(<ActivityImageGallery activityId={4} />);
    expect(screen.getByLabelText(/loading photos/i)).toBeInTheDocument();
  });

  it('shows an error when loading images fails', async () => {
    getActivityImages.mockRejectedValue(new Error('load failed'));
    render(<ActivityImageGallery activityId={4} editable />);
    expect(await screen.findByText('Could not load photos.')).toBeInTheDocument();
  });

  it('uploads every selected photo then reloads and notifies the parent', async () => {
    const onChanged = vi.fn();
    const first = new File(['a'], 'a.jpg', { type: 'image/jpeg' });
    const second = new File(['b'], 'b.png', { type: 'image/png' });
    render(<ActivityImageGallery activityId={4} editable onChanged={onChanged} />);

    await screen.findByRole('img', { name: /activity photo/i });
    const input = screen.getByLabelText(/add photos/i);
    const inputClick = vi.fn();
    input.addEventListener('click', inputClick);
    fireEvent.click(screen.getByRole('button', { name: /add photos/i }));
    expect(inputClick).toHaveBeenCalledTimes(1);
    fireEvent.change(input, { target: { files: [first, second] } });

    await waitFor(() => {
      expect(uploadActivityImage).toHaveBeenCalledWith(4, first);
      expect(uploadActivityImage).toHaveBeenCalledWith(4, second);
      expect(getActivityImages).toHaveBeenCalledTimes(2);
      expect(onChanged).toHaveBeenCalledTimes(1);
    });
  });

  it('shows an error when an upload fails', async () => {
    uploadActivityImage.mockRejectedValue(new Error('upload failed'));
    render(<ActivityImageGallery activityId={4} editable />);

    const input = await screen.findByLabelText(/add photos/i);
    fireEvent.change(input, {
      target: { files: [new File(['a'], 'a.jpg', { type: 'image/jpeg' })] },
    });

    expect(await screen.findByText('Could not upload the photo.')).toBeInTheDocument();
  });

  it('shows an error when deleting fails', async () => {
    deleteActivityImage.mockRejectedValue(new Error('delete failed'));
    render(<ActivityImageGallery activityId={4} editable />);
    fireEvent.click(await screen.findByRole('button', { name: /delete photo/i }));
    expect(await screen.findByText('Could not delete the photo.')).toBeInTheDocument();
  });

  it('opens the full image in a lightbox and closes it', async () => {
    render(<ActivityImageGallery activityId={4} />);
    fireEvent.click(await screen.findByRole('img', { name: /activity photo/i }));

    const dialog = await screen.findByRole('dialog');
    const full = within(dialog).getByRole('img', { name: /activity photo/i });
    const src = full.getAttribute('src') ?? '';
    expect(new URL(src, window.location.href).pathname).toBe('/api/v1/activities/4/images/9/file');

    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('has no delete control on the read-only surface', async () => {
    render(<ActivityImageGallery activityId={4} />);
    await screen.findByRole('img', { name: /activity photo/i });
    expect(screen.queryByRole('button', { name: /delete photo/i })).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/add photos/i)).not.toBeInTheDocument();
  });
});
