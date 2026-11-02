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
  it('shows a load error even on a read-only gallery', async () => {
    getNoteImages.mockRejectedValue(new Error('down'));
    render(<NoteImageGallery noteId={4} />);
    expect(await screen.findByText('Could not load photos.')).toBeTruthy();
  });

  it('uploads selected files, reloads and notifies the parent', async () => {
    const onChanged = vi.fn();
    const { container } = render(<NoteImageGallery noteId={4} editable onChanged={onChanged} />);
    await screen.findByRole('img', { name: /note photo/i });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' });
    fireEvent.change(input, { target: { files: [file] } });
    await waitFor(() => expect(uploadNoteImage).toHaveBeenCalledWith(4, file));
    await waitFor(() => expect(getNoteImages).toHaveBeenCalledTimes(2));
    expect(onChanged).toHaveBeenCalled();
  });

  it('shows an upload error', async () => {
    uploadNoteImage.mockRejectedValue(new Error('upload failed'));
    const { container } = render(<NoteImageGallery noteId={4} editable />);
    await screen.findByRole('img', { name: /note photo/i });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, {
      target: { files: [new File(['x'], 'photo.jpg', { type: 'image/jpeg' })] },
    });
    expect(await screen.findByText('Could not upload the photo.')).toBeTruthy();
  });

  it('shows a delete error', async () => {
    deleteNoteImage.mockRejectedValue(new Error('delete failed'));
    render(<NoteImageGallery noteId={4} editable />);
    fireEvent.click(await screen.findByRole('button', { name: /delete photo/i }));
    expect(await screen.findByText('Could not delete the photo.')).toBeTruthy();
  });

  it('opens the lightbox and shows the full image', async () => {
    render(<NoteImageGallery noteId={4} />);
    fireEvent.click(await screen.findByRole('img', { name: /note photo/i }));
    expect(screen.getByRole('dialog')).toBeTruthy();
  });

  it('offers an add button that opens the file picker on an editable empty gallery', async () => {
    getNoteImages.mockResolvedValue({ images: [], total: 0 });
    const { container } = render(<NoteImageGallery noteId={4} editable />);
    const button = await screen.findByRole('button', { name: 'Add photos' });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const click = vi.spyOn(input, 'click').mockImplementation(() => {});
    fireEvent.click(button);
    expect(click).toHaveBeenCalled();
  });
});
