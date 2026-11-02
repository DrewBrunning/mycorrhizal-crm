import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
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
});
