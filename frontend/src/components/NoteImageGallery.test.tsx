import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import NoteImageGallery from './NoteImageGallery';

const getNoteImages = vi.fn();
const deleteNoteImage = vi.fn();
const uploadNoteImage = vi.fn();

vi.mock('../api/notes', async () => {
  const actual = await vi.importActual<typeof import('../api/notes')>('../api/notes');
  return {
    ...actual,
    getNoteImages: (...args: unknown[]) => getNoteImages(...args),
    deleteNoteImage: (...args: unknown[]) => deleteNoteImage(...args),
    uploadNoteImage: (...args: unknown[]) => uploadNoteImage(...args),
  };
});

const image = {
  id: 9,
  uuid: '0f0e0d0c-0b0a-4908-8706-050403020100',
  note_id: 4,
  mime_type: 'image/jpeg' as const,
  width: 100,
  height: 80,
  size_bytes: 1200,
  sensitivity: 'normal' as const,
  taken_at: null,
  taken_date: null,
  created_at: '2026-01-01T00:00:00Z',
  file_url: '/api/v1/notes/4/images/9/file',
  thumbnail_url: '/api/v1/notes/4/images/9/thumbnail',
};

describe('NoteImageGallery', () => {
  beforeEach(() => {
    getNoteImages.mockReset().mockResolvedValue({ images: [image], total: 1 });
    deleteNoteImage.mockReset().mockResolvedValue(undefined);
    uploadNoteImage.mockReset().mockResolvedValue(image);
  });

  afterEach(cleanup);

  it('renders thumbnails from the owner-nested endpoint', async () => {
    render(<NoteImageGallery noteId={4} />);
    const thumb = await screen.findByRole('img', { name: /note photo/i });
    expect(thumb.getAttribute('src')).toMatch(/\/api\/v1\/notes\/4\/images\/9\/thumbnail$/);
  });

  it('deletes a photo from the editable surface', async () => {
    render(<NoteImageGallery noteId={4} editable />);
    fireEvent.click(await screen.findByRole('button', { name: /delete photo/i }));
    await waitFor(() => expect(deleteNoteImage).toHaveBeenCalledWith(4, 9));
  });

  it('renders nothing read-only when there are no images', async () => {
    getNoteImages.mockResolvedValue({ images: [], total: 0 });
    const { container } = render(<NoteImageGallery noteId={4} />);
    await waitFor(() => expect(getNoteImages).toHaveBeenCalledWith(4));
    expect(container).toBeEmptyDOMElement();
  });
});
