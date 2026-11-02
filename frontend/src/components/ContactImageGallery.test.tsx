import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ContactImageGallery from './ContactImageGallery';

const getContactImages = vi.fn();
const deleteContactImage = vi.fn();
const uploadContactImage = vi.fn();

vi.mock('../api/contactImages', async () => {
  const actual =
    await vi.importActual<typeof import('../api/contactImages')>('../api/contactImages');
  return {
    ...actual,
    getContactImages: (...args: unknown[]) => getContactImages(...args),
    deleteContactImage: (...args: unknown[]) => deleteContactImage(...args),
    uploadContactImage: (...args: unknown[]) => uploadContactImage(...args),
  };
});

const image = {
  id: 9,
  uuid: '0f0e0d0c-0b0a-4908-8706-050403020100',
  contact_id: 4,
  mime_type: 'image/jpeg' as const,
  width: 100,
  height: 80,
  size_bytes: 1200,
  sensitivity: 'normal' as const,
  taken_at: null,
  taken_date: null,
  created_at: '2026-01-01T00:00:00Z',
  file_url: '/api/v1/contacts/4/images/9/file',
  thumbnail_url: '/api/v1/contacts/4/images/9/thumbnail',
};

describe('ContactImageGallery', () => {
  beforeEach(() => {
    getContactImages.mockReset().mockResolvedValue({ images: [image], total: 1 });
    deleteContactImage.mockReset().mockResolvedValue(undefined);
    uploadContactImage.mockReset().mockResolvedValue(image);
  });

  afterEach(cleanup);

  it('renders thumbnails from the owner-nested endpoint', async () => {
    render(<ContactImageGallery contactId={4} />);
    const thumb = await screen.findByRole('img', { name: /contact photo/i });
    expect(thumb.getAttribute('src')).toMatch(/\/api\/v1\/contacts\/4\/images\/9\/thumbnail$/);
  });

  it('deletes a photo from the editable surface', async () => {
    render(<ContactImageGallery contactId={4} editable />);
    fireEvent.click(await screen.findByRole('button', { name: /delete photo/i }));
    await waitFor(() => expect(deleteContactImage).toHaveBeenCalledWith(4, 9));
  });

  it('renders nothing read-only when there are no images', async () => {
    getContactImages.mockResolvedValue({ images: [], total: 0 });
    const { container } = render(<ContactImageGallery contactId={4} />);
    await waitFor(() => expect(getContactImages).toHaveBeenCalledWith(4));
    expect(container).toBeEmptyDOMElement();
  });
});
