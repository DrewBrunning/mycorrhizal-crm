import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import AttachmentsSection from './AttachmentsSection';
import { SnackbarProvider } from '../context/SnackbarContext';
import { listContactAttachments, uploadAttachment, deleteAttachment, attachmentDownloadUrl } from '../api/attachments';

afterEach(cleanup);

vi.mock('../api/attachments', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/attachments')>();
  return {
    ...actual,
    listContactAttachments: vi.fn(),
    uploadAttachment: vi.fn(),
    deleteAttachment: vi.fn(),
  };
});

beforeEach(() => {
  vi.mocked(listContactAttachments).mockReset();
  vi.mocked(uploadAttachment).mockReset();
  vi.mocked(deleteAttachment).mockReset();
});

function renderSection() {
  return render(
    <SnackbarProvider>
      <AttachmentsSection contactId={7} />
    </SnackbarProvider>
  );
}

const sampleAttachment = {
  ID: 1,
  CreatedAt: '2026-01-01T00:00:00Z',
  UpdatedAt: '2026-01-01T00:00:00Z',
  contact_vcard_uid: 'uid-1',
  original_name: 'cv.pdf',
  content_type: 'application/pdf',
  size_bytes: 2048,
};

test('lists attachments with size and type', async () => {
  vi.mocked(listContactAttachments).mockResolvedValue({ attachments: [sampleAttachment], total: 1 });
  renderSection();

  expect(await screen.findByText('cv.pdf')).toBeInTheDocument();
  expect(screen.getByText('2.0 KB · application/pdf')).toBeInTheDocument();
  expect(listContactAttachments).toHaveBeenCalledWith(7);
});

test('shows the empty state', async () => {
  vi.mocked(listContactAttachments).mockResolvedValue({ attachments: [], total: 0 });
  renderSection();
  expect(await screen.findByText('No attachments yet.')).toBeInTheDocument();
});

test('uploads a file via the hidden input and refreshes the list', async () => {
  vi.mocked(listContactAttachments).mockResolvedValue({ attachments: [], total: 0 });
  vi.mocked(uploadAttachment).mockResolvedValue(sampleAttachment);
  renderSection();
  await screen.findByText('No attachments yet.');

  const input = document.querySelector('input[type="file"]') as HTMLInputElement;
  Object.defineProperty(input, 'files', { value: [new File(['pdf'], 'new.pdf', { type: 'application/pdf' })], configurable: true });
  fireEvent.change(input);

  await waitFor(() => expect(uploadAttachment).toHaveBeenCalledWith(7, expect.any(File)));
  expect(listContactAttachments).toHaveBeenCalledTimes(2);
});

test('download link points at the API endpoint', async () => {
  vi.mocked(listContactAttachments).mockResolvedValue({ attachments: [sampleAttachment], total: 1 });
  renderSection();
  await screen.findByText('cv.pdf');

  const download = screen.getByLabelText('Download');
  expect(download).toHaveAttribute('href', attachmentDownloadUrl(sampleAttachment.ID));
});

test('delete removes the attachment after confirmation', async () => {
  vi.mocked(listContactAttachments).mockResolvedValue({ attachments: [sampleAttachment], total: 1 });
  vi.mocked(deleteAttachment).mockResolvedValue(undefined);
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderSection();
  await screen.findByText('cv.pdf');

  fireEvent.click(screen.getByLabelText('Delete'));
  await waitFor(() => expect(deleteAttachment).toHaveBeenCalledWith(1));
  expect(screen.queryByText('cv.pdf')).not.toBeInTheDocument();
  confirmSpy.mockRestore();
});

test('delete without confirmation does not call the API', async () => {
  vi.mocked(listContactAttachments).mockResolvedValue({ attachments: [sampleAttachment], total: 1 });
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  renderSection();
  await screen.findByText('cv.pdf');

  fireEvent.click(screen.getByLabelText('Delete'));
  expect(deleteAttachment).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});
