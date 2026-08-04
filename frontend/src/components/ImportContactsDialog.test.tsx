import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ImportContactsDialog from './ImportContactsDialog';
import { SnackbarProvider } from '../context/SnackbarContext';
import {
  uploadCSVForImport,
  uploadVCFForImport,
  getImportPreview,
  confirmImport,
  confirmVCFImport,
} from '../api/import';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/import', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/import')>();
  return {
    ...actual,
    uploadCSVForImport: vi.fn(),
    uploadVCFForImport: vi.fn(),
    getImportPreview: vi.fn(),
    confirmImport: vi.fn(),
    confirmVCFImport: vi.fn(),
  };
});

beforeEach(() => {
  vi.mocked(uploadCSVForImport).mockReset();
  vi.mocked(uploadVCFForImport).mockReset();
  vi.mocked(getImportPreview).mockReset();
  vi.mocked(confirmImport).mockReset();
  vi.mocked(confirmVCFImport).mockReset();
});

function renderDialog(props: Partial<React.ComponentProps<typeof ImportContactsDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ImportContactsDialog> = {
    open: true,
    onClose: vi.fn(),
    onImportComplete: vi.fn(),
    ...props,
  };
  return render(
    <SnackbarProvider>
      <ImportContactsDialog {...defaults} />
    </SnackbarProvider>
  );
}

// Simulates picking a file through the hidden <input type="file"> — this
// codebase doesn't have @testing-library/user-event, so the file list is
// set directly the way RTL's own file-input recipe does.
function selectFile(file: File) {
  const input = document.getElementById('import-file-input') as HTMLInputElement;
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  fireEvent.change(input);
}

test('shows the upload dropzone by default', () => {
  renderDialog();
  expect(screen.getByText(/drag and drop a csv file/i)).toBeInTheDocument();
});

test('rejects an unsupported file extension before calling the API', async () => {
  renderDialog();
  const file = new File(['not a contact'], 'notes.txt', { type: 'text/plain' });
  selectFile(file);

  await waitFor(() => expect(screen.getByText(/please select a valid csv file/i)).toBeInTheDocument());
  expect(uploadCSVForImport).not.toHaveBeenCalled();
  expect(uploadVCFForImport).not.toHaveBeenCalled();
});

test('CSV upload walks through mapping, preview, and confirm', async () => {
  vi.mocked(uploadCSVForImport).mockResolvedValue({
    session_id: 'sess-1',
    headers: ['Name', 'Email'],
    suggested_mappings: [
      { csv_column: 'Name', contact_field: 'firstname', group: 0 },
      { csv_column: 'Email', contact_field: 'email', group: 0 },
    ],
    row_count: 1,
    sample_data: [['Ada Lovelace', 'ada@example.com']],
  });
  vi.mocked(getImportPreview).mockResolvedValue({
    session_id: 'sess-1',
    rows: [
      {
        row_index: 0,
        parsed_contact: { firstname: 'Ada', lastname: '', email: 'ada@example.com' },
        validation_errors: [],
        duplicate_match: null,
        suggested_action: 'add',
      },
    ],
    total_rows: 1,
    valid_rows: 1,
    duplicate_count: 0,
    error_count: 0,
  });
  vi.mocked(confirmImport).mockResolvedValue({
    total_processed: 1,
    created: 1,
    updated: 0,
    skipped: 0,
    errors: [],
  });

  const onImportComplete = vi.fn();
  renderDialog({ onImportComplete });

  selectFile(new File(['Name,Email\nAda Lovelace,ada@example.com'], 'contacts.csv', { type: 'text/csv' }));

  // Mapping step: the CSV column headers and suggested mappings render.
  await waitFor(() => expect(screen.getByText('Name')).toBeInTheDocument());
  expect(screen.getAllByText('Email').length).toBeGreaterThanOrEqual(1);
  expect(uploadCSVForImport).toHaveBeenCalledTimes(1);

  fireEvent.click(screen.getByRole('button', { name: /continue/i }));

  // Preview step: summary chips reflect the one row to create.
  await waitFor(() => expect(screen.getByText('1 to create')).toBeInTheDocument());
  expect(getImportPreview).toHaveBeenCalledWith('sess-1', [
    { csv_column: 'Name', contact_field: 'firstname', group: 0 },
    { csv_column: 'Email', contact_field: 'email', group: 0 },
  ]);

  fireEvent.click(screen.getByRole('button', { name: /^import$/i }));

  // Result step: created/updated/skipped counts, and the caller is notified.
  await waitFor(() => expect(screen.getByText('1 contacts created')).toBeInTheDocument());
  expect(confirmImport).toHaveBeenCalledWith('sess-1', [{ row_index: 0, action: 'add' }]);
  expect(onImportComplete).toHaveBeenCalled();
});

test('mapping step blocks Continue until at least one column is mapped', async () => {
  vi.mocked(uploadCSVForImport).mockResolvedValue({
    session_id: 'sess-2',
    headers: ['Col A'],
    suggested_mappings: [{ csv_column: 'Col A', contact_field: '', group: 0 }],
    row_count: 1,
    sample_data: [['x']],
  });

  renderDialog();
  selectFile(new File(['Col A\nx'], 'contacts.csv', { type: 'text/csv' }));

  await waitFor(() => expect(screen.getByText('Col A')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: /continue/i }));

  await waitFor(() => expect(screen.getByText(/please map at least one column/i)).toBeInTheDocument());
  expect(getImportPreview).not.toHaveBeenCalled();
});

test('VCF upload skips the mapping step and goes straight to preview', async () => {
  vi.mocked(uploadVCFForImport).mockResolvedValue({
    session_id: 'sess-vcf',
    rows: [
      {
        row_index: 0,
        parsed_contact: { firstname: 'Grace', lastname: 'Hopper', email: '' },
        validation_errors: [],
        duplicate_match: null,
        suggested_action: 'add',
      },
    ],
    total_rows: 1,
    valid_rows: 1,
    duplicate_count: 0,
    error_count: 0,
  });

  renderDialog();
  selectFile(new File(['BEGIN:VCARD\nEND:VCARD'], 'contact.vcf', { type: 'text/vcard' }));

  await waitFor(() => expect(screen.getByText('1 to create')).toBeInTheDocument());
  expect(uploadVCFForImport).toHaveBeenCalledTimes(1);
  expect(getImportPreview).not.toHaveBeenCalled();
});

test('a duplicate row defaults to "update existing" and reports the match', async () => {
  vi.mocked(uploadVCFForImport).mockResolvedValue({
    session_id: 'sess-dup',
    rows: [
      {
        row_index: 0,
        parsed_contact: { firstname: 'Bob', lastname: 'Smith', email: 'bob@example.com' },
        validation_errors: [],
        duplicate_match: {
          existing_contact_id: 9,
          existing_firstname: 'Bob',
          existing_lastname: 'Smith',
          existing_email: 'bob@example.com',
          match_reason: 'email',
        },
        suggested_action: 'update',
      },
    ],
    total_rows: 1,
    valid_rows: 1,
    duplicate_count: 1,
    error_count: 0,
  });

  renderDialog();
  selectFile(new File(['BEGIN:VCARD\nEND:VCARD'], 'contact.vcf', { type: 'text/vcard' }));

  await waitFor(() => expect(screen.getByText('1 to update')).toBeInTheDocument());
});

test('an upload failure surfaces the error without advancing the step', async () => {
  vi.mocked(uploadCSVForImport).mockRejectedValue(new Error('server exploded'));

  renderDialog();
  selectFile(new File(['a,b'], 'contacts.csv', { type: 'text/csv' }));

  await waitFor(() => expect(screen.getByText('server exploded')).toBeInTheDocument());
  // Still on the upload step — the dropzone is still showing.
  expect(screen.getByText(/drag and drop a csv file/i)).toBeInTheDocument();
});
