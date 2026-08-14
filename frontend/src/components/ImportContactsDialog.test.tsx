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
  ImportRowPreview,
  DuplicateMatch,
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

// T96: the backend always emits merge_diff/batch_duplicate_of on preview rows
// (the Go struct has no omitempty on them), so every fixture carries them —
// mirroring what a real ImportPreviewResponse looks like on the wire.
function row(overrides: Partial<ImportRowPreview> & { parsed_contact: ImportRowPreview['parsed_contact'] }): ImportRowPreview {
  return {
    row_index: 0,
    validation_errors: [],
    duplicate_match: null,
    suggested_action: 'add',
    merge_diff: null,
    batch_duplicate_of: null,
    ...overrides,
  };
}

function dupMatch(overrides: Partial<DuplicateMatch>): DuplicateMatch {
  return {
    existing_contact_id: 9,
    existing_firstname: '',
    existing_lastname: '',
    existing_email: '',
    existing_phone: '',
    match_reason: 'email',
    ...overrides,
  };
}

// Loads a VCF-based preview with the given rows.
async function loadPreview(rows: ImportRowPreview[]) {
  vi.mocked(uploadVCFForImport).mockResolvedValue({
    session_id: 'sess-test',
    rows,
    total_rows: rows.length,
    valid_rows: rows.filter((r) => r.validation_errors.length === 0).length,
    duplicate_count: rows.filter((r) => r.duplicate_match).length,
    error_count: rows.filter((r) => r.validation_errors.length > 0).length,
  });
  renderDialog();
  selectFile(new File(['BEGIN:VCARD\nEND:VCARD'], 'contact.vcf', { type: 'text/vcard' }));
  await waitFor(() => expect(screen.getByText('Resolve all as merged')).toBeInTheDocument());
}

test('shows the upload dropzone by default', () => {
  renderDialog();
  expect(screen.getByText(/drag and drop a csv or vcf file/i)).toBeInTheDocument();
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
      row({ parsed_contact: { firstname: 'Ada', lastname: '', email: 'ada@example.com' } }),
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

  fireEvent.click(screen.getByRole('button', { name: /apply decisions/i }));

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
    rows: [row({ parsed_contact: { firstname: 'Grace', lastname: 'Hopper', email: '' } })],
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

// T96: a row that matches an existing record shows the match, defaults to
// Merge, and the conflict heading counts it as one remaining.
test('a duplicate row defaults to Merge, shows the match, and renders the diff', async () => {
  await loadPreview([
    row({
      parsed_contact: { firstname: 'Bob', lastname: 'Smith', email: 'bob@example.com' },
      duplicate_match: dupMatch({ existing_firstname: 'Bob', existing_lastname: 'Smith', existing_email: 'bob@example.com' }),
      suggested_action: 'update',
      merge_diff: {
        updated: [{ field: 'job_title', label: 'Job Title', old: 'Engineer', new: 'Staff Engineer' }],
        added: [{ kind: 'phone', value: '+15559998888' }],
      },
    }),
  ]);

  // The match line and the conflict heading.
  expect(screen.getByText(/Matches: Bob Smith/)).toBeInTheDocument();
  expect(screen.getByText('Resolve Conflicts (1 remaining)')).toBeInTheDocument();

  // The diff describes exactly what Merge will do.
  expect(screen.getByText(/\+ new phone: \+15559998888/)).toBeInTheDocument();
  expect(screen.getByText('Job Title: Engineer → Staff Engineer')).toBeInTheDocument();

  // The row defaults to Merge, and Apply Decisions reports one decision.
  expect(screen.getByRole('button', { name: /apply decisions \(1\)/i })).toBeInTheDocument();
  expect(screen.getByText('1 to update')).toBeInTheDocument();
});

test('resolving every conflict zeroes the remaining count', async () => {
  await loadPreview([
    row({
      parsed_contact: { firstname: 'Bob', lastname: 'Smith', email: 'bob@example.com' },
      duplicate_match: dupMatch({ existing_firstname: 'Bob', existing_lastname: 'Smith', existing_email: 'bob@example.com' }),
      suggested_action: 'update',
      merge_diff: { updated: [], added: [] },
    }),
  ]);
  expect(screen.getByText('Resolve Conflicts (1 remaining)')).toBeInTheDocument();

  // Explicitly choose Discard New: the conflict is now resolved.
  fireEvent.click(screen.getByRole('button', { name: /discard new/i }));
  expect(screen.getByText('1 to skip')).toBeInTheDocument();
  expect(screen.getByText(/no duplicate matches/i)).toBeInTheDocument();
});

// T96: a row that duplicates an EARLIER row of the same import is flagged with
// the source row, defaults to Discard New, and has no Merge button (there is
// no existing record to merge into).
test('within-batch duplicates are flagged, default to Discard, and cannot merge', async () => {
  await loadPreview([
    row({ parsed_contact: { firstname: 'Jane', lastname: 'Smith', email: 'jane@example.com' } }),
    row({
      row_index: 1,
      parsed_contact: { firstname: 'Jane', lastname: 'Smith', email: 'jane@example.com' },
      suggested_action: 'skip',
      batch_duplicate_of: 0,
    }),
  ]);

  expect(screen.getByText('Duplicates row 1 of this import')).toBeInTheDocument();
  // The twin defaults to Discard New; the first occurrence stays Keep Both.
  expect(screen.getByText('1 to create')).toBeInTheDocument();
  expect(screen.getByText('1 to skip')).toBeInTheDocument();

  const mergeButtons = screen.getAllByRole('button', { name: /^merge$/i });
  // Row 1 (batch dup) has no mergeable target; row 0 (new) also has none —
  // both Merge buttons must be disabled.
  expect(mergeButtons.length).toBeGreaterThan(0);
  mergeButtons.forEach((b) => expect(b).toBeDisabled());
});

test('an upload failure surfaces the error without advancing the step', async () => {
  vi.mocked(uploadCSVForImport).mockRejectedValue(new Error('server exploded'));

  renderDialog();
  selectFile(new File(['a,b'], 'contacts.csv', { type: 'text/csv' }));

  await waitFor(() => expect(screen.getByText('server exploded')).toBeInTheDocument());
  // Still on the upload step — the dropzone is still showing.
  expect(screen.getByText(/drag and drop a csv or vcf file/i)).toBeInTheDocument();
});

// T56 bulk controls, renamed for T96: "Resolve all as merged" applies each
// valid row's own suggested action (add for new, merge for duplicates, skip
// for within-batch duplicates) in one click, and "Skip all" marks every valid
// row as skip — both leave errored rows alone.
test('resolve all as merged applies each row suggested action', async () => {
  vi.mocked(uploadVCFForImport).mockResolvedValue({
    session_id: 'sess-bulk',
    rows: [
      row({ parsed_contact: { firstname: 'New', lastname: 'One', email: '' } }),
      row({
        row_index: 1,
        parsed_contact: { firstname: 'Dup', lastname: 'Two', email: '' },
        duplicate_match: dupMatch({ existing_firstname: 'Dup', existing_lastname: 'Two', match_reason: 'name' }),
        suggested_action: 'update',
      }),
      row({
        row_index: 2,
        parsed_contact: { firstname: 'Bad', lastname: '', email: 'not-an-email' },
        validation_errors: ['bad email'],
        suggested_action: 'skip',
      }),
    ],
    total_rows: 3,
    valid_rows: 2,
    duplicate_count: 1,
    error_count: 1,
  });
  vi.mocked(confirmVCFImport).mockResolvedValue({ total_processed: 2, created: 1, updated: 1, skipped: 0, errors: [] });

  renderDialog();
  selectFile(new File(['BEGIN:VCARD\nEND:VCARD'], 'contact.vcf', { type: 'text/vcard' }));
  await waitFor(() => expect(screen.getByText('1 to create')).toBeInTheDocument());

  // First flatten every valid row to skip, then Resolve all must revert them
  // to their suggested actions — proving the bulk action actually rewrites
  // the per-row state rather than being a no-op on the initial defaults.
  fireEvent.click(screen.getByRole('button', { name: /skip all/i }));
  expect(screen.getByText('3 to skip')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /resolve all as merged/i }));
  fireEvent.click(screen.getByRole('button', { name: /apply decisions/i }));

  await waitFor(() => expect(confirmVCFImport).toHaveBeenCalled());
  const actions = vi.mocked(confirmVCFImport).mock.calls[0][1];
  expect(actions).toEqual([
    { row_index: 0, action: 'add' },
    { row_index: 1, action: 'update' },
    { row_index: 2, action: 'skip' },
  ]);
});

test('skip all marks every valid row as skip', async () => {
  vi.mocked(uploadVCFForImport).mockResolvedValue({
    session_id: 'sess-skipall',
    rows: [
      row({ parsed_contact: { firstname: 'A', lastname: '', email: '' } }),
      row({ row_index: 1, parsed_contact: { firstname: 'B', lastname: '', email: '' } }),
    ],
    total_rows: 2,
    valid_rows: 2,
    duplicate_count: 0,
    error_count: 0,
  });
  vi.mocked(confirmVCFImport).mockResolvedValue({ total_processed: 2, created: 0, updated: 0, skipped: 2, errors: [] });

  renderDialog();
  selectFile(new File(['BEGIN:VCARD\nEND:VCARD'], 'contact.vcf', { type: 'text/vcard' }));
  await waitFor(() => expect(screen.getByText('2 to create')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /skip all/i }));
  fireEvent.click(screen.getByRole('button', { name: /apply decisions/i }));

  await waitFor(() => expect(confirmVCFImport).toHaveBeenCalled());
  const actions = vi.mocked(confirmVCFImport).mock.calls[0][1];
  expect(actions).toEqual([
    { row_index: 0, action: 'skip' },
    { row_index: 1, action: 'skip' },
  ]);
});

// T56: the preview is paginated client-side, so a full address-book import
// mounts only one page of decision cards at a time.
test('paginates a preview larger than one page', async () => {
  const rows = Array.from({ length: 45 }, (_, i) =>
    row({ row_index: i, parsed_contact: { firstname: `C${i}`, lastname: '', email: '' } })
  );
  vi.mocked(uploadVCFForImport).mockResolvedValue({
    session_id: 'sess-page',
    rows,
    total_rows: 45,
    valid_rows: 45,
    duplicate_count: 0,
    error_count: 0,
  });

  renderDialog();
  selectFile(new File(['BEGIN:VCARD\nEND:VCARD'], 'contact.vcf', { type: 'text/vcard' }));

  await waitFor(() => expect(screen.getByText('45 to create')).toBeInTheDocument());
  // Only the first page's rows render.
  expect(screen.getByText('C0')).toBeInTheDocument();
  expect(screen.queryByText('C20')).not.toBeInTheDocument();

  // Advance to page 2 and confirm the next window renders.
  fireEvent.click(screen.getByRole('button', { name: /next page/i }));
  await waitFor(() => expect(screen.getByText('C20')).toBeInTheDocument());
  expect(screen.queryByText('C0')).not.toBeInTheDocument();
});
