import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { suggestContactAddresses } from './api/dataSuggestions';
import { exportContacts, exportContactsAsVcf, exportDataAsCsv } from './api/export';
import { getImportHistory } from './api/import';
import { suggestRelationshipEdges } from './api/relationshipEdges';
import DataSettingsPage from './DataSettingsPage';

// This codebase's vitest has no auto-cleanup (CLAUDE.md frontend trap #1).
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

// The page composes several settings panels that each fetch on mount; stub
// them so this test only exercises the issue #651 import-history table. The
// three import dialogs and the export field picker are stubbed as minimal
// interactive stand-ins (rather than always-null) so tests can exercise the
// open/close/complete callbacks the page wires them to.
vi.mock('./components/CalendarSyncSettings', () => ({ default: () => null }));
vi.mock('./components/ContactAddressSuggestions', () => ({ default: () => null }));
vi.mock('./components/ContactFieldSettings', () => ({ default: () => null }));
vi.mock('./components/CustomFieldsSettings', () => ({ default: () => null }));
vi.mock('./components/ExportFieldPickerDialog', () => ({
  default: (props: {
    open: boolean;
    onClose: () => void;
    onExport: (
      format: 'vcf4',
      selection: { sections: string[]; includeSensitive: boolean },
    ) => void;
  }) =>
    props.open ? (
      <div>
        <button type="button" onClick={props.onClose}>
          picker-close
        </button>
        <button
          type="button"
          onClick={() => props.onExport('vcf4', { sections: ['name'], includeSensitive: false })}
        >
          picker-export
        </button>
      </div>
    ) : null,
}));
vi.mock('./components/ImportContactsDialog', () => ({
  default: (props: { open: boolean; onClose: () => void; onImportComplete: () => void }) =>
    props.open ? (
      <div>
        <button type="button" onClick={props.onClose}>
          import-close
        </button>
        <button type="button" onClick={props.onImportComplete}>
          import-complete
        </button>
      </div>
    ) : null,
}));
vi.mock('./components/MonicaImportDialog', () => ({
  default: (props: { open: boolean; onClose: () => void; onImportComplete: () => void }) =>
    props.open ? (
      <div>
        <button type="button" onClick={props.onClose}>
          monica-close
        </button>
        <button type="button" onClick={props.onImportComplete}>
          monica-complete
        </button>
      </div>
    ) : null,
}));
vi.mock('./components/MeerkatImportDialog', () => ({
  default: (props: { open: boolean; onClose: () => void; onImportComplete: () => void }) =>
    props.open ? (
      <div>
        <button type="button" onClick={props.onClose}>
          meerkat-close
        </button>
        <button type="button" onClick={props.onImportComplete}>
          meerkat-complete
        </button>
      </div>
    ) : null,
}));
vi.mock('./components/RelationshipSuggestionsInbox', () => ({ default: () => null }));

vi.mock('./api/import', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/import')>();
  return { ...actual, getImportHistory: vi.fn() };
});

vi.mock('./api/relationshipEdges', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/relationshipEdges')>();
  return { ...actual, suggestRelationshipEdges: vi.fn() };
});

vi.mock('./api/dataSuggestions', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/dataSuggestions')>();
  return { ...actual, suggestContactAddresses: vi.fn() };
});

vi.mock('./api/export', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/export')>();
  return {
    ...actual,
    exportDataAsCsv: vi.fn(),
    exportContactsAsVcf: vi.fn(),
    exportContacts: vi.fn(),
  };
});

const historyMock = vi.mocked(getImportHistory);
const suggestRelationshipsMock = vi.mocked(suggestRelationshipEdges);
const suggestAddressesMock = vi.mocked(suggestContactAddresses);
const exportCsvMock = vi.mocked(exportDataAsCsv);
const exportVcfMock = vi.mocked(exportContactsAsVcf);
const exportCustomMock = vi.mocked(exportContacts);

beforeEach(() => {
  historyMock.mockReset();
  historyMock.mockResolvedValue([]);
  suggestRelationshipsMock.mockReset();
  suggestAddressesMock.mockReset();
  exportCsvMock.mockReset();
  exportVcfMock.mockReset();
  exportCustomMock.mockReset();
});

function renderPage() {
  return render(
    <MemoryRouter>
      <DataSettingsPage />
    </MemoryRouter>,
  );
}

test('shows the empty state when there is no import history', async () => {
  historyMock.mockResolvedValue([]);
  renderPage();

  expect(await screen.findByText('No imports yet.')).toBeInTheDocument();
  expect(historyMock).toHaveBeenCalledTimes(1);
});

test('renders one row per import run with its counts', async () => {
  historyMock.mockResolvedValue([
    {
      id: 7,
      format: 'jscontact',
      total_processed: 9,
      created: 5,
      updated: 3,
      skipped: 1,
      error_count: 2,
      created_at: '2026-08-27T12:00:00Z',
    },
  ]);
  renderPage();

  const cell = await screen.findByText('JSContact');
  const row = cell.closest('tr');
  expect(row).not.toBeNull();
  // created / updated / skipped / errors
  expect(row).toHaveTextContent('5');
  expect(row).toHaveTextContent('3');
  expect(row).toHaveTextContent('1');
  expect(row).toHaveTextContent('2');
});

test('surfaces a load error', async () => {
  historyMock.mockRejectedValue(new Error('boom'));
  renderPage();

  await waitFor(() => expect(screen.getByText('boom')).toBeInTheDocument());
});

test('offers the Monica import assistant alongside the file import (issue #549)', async () => {
  historyMock.mockResolvedValue([]);
  renderPage();

  expect(await screen.findByRole('button', { name: 'Import from Monica' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Import from Meerkat' })).toBeInTheDocument();
});

// Issue #861: the CSV export is the user's own full backup, so it carries the
// private/secret items and unconfirmed relationships the vCard/JSContact
// exports hold back. A user who reads "private" as "not in my backup file"
// would hand out more than they meant to, so the panel must say what the file
// contains *before* the download button, not in a doc they never open.
test('warns that the CSV backup includes private and secret data before downloading', async () => {
  historyMock.mockResolvedValue([]);
  renderPage();

  const notice = await screen.findByText(/private or secret/i);
  expect(notice).toBeInTheDocument();
  expect(notice).toHaveTextContent(/vCard or JSContact/i);

  // Positioned ahead of the control it qualifies.
  const download = screen.getByRole('button', { name: 'Download CSV' });
  expect(notice.compareDocumentPosition(download) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test('suggesting relationships reports how many new suggestions were generated', async () => {
  suggestRelationshipsMock.mockResolvedValue({ message: 'ok', suggested_edges: [], total: 3 });
  renderPage();

  const button = await screen.findByRole('button', { name: 'Suggest relationships' });
  fireEvent.click(button);

  expect(
    await screen.findByText('3 new relationship suggestions generated — review them below.'),
  ).toBeInTheDocument();
  expect(suggestRelationshipsMock).toHaveBeenCalledTimes(1);
});

test('suggesting relationships reports when nothing new was found', async () => {
  suggestRelationshipsMock.mockResolvedValue({ message: 'ok', suggested_edges: [], total: 0 });
  renderPage();

  const button = await screen.findByRole('button', { name: 'Suggest relationships' });
  fireEvent.click(button);

  expect(await screen.findByText('No new relationship suggestions right now.')).toBeInTheDocument();
});

test('surfaces an error when suggesting relationships fails', async () => {
  suggestRelationshipsMock.mockRejectedValue(new Error('network down'));
  renderPage();

  const button = await screen.findByRole('button', { name: 'Suggest relationships' });
  fireEvent.click(button);

  expect(
    await screen.findByText('Could not generate relationship suggestions.'),
  ).toBeInTheDocument();
});

test('suggesting addresses reports how many suggestions were found', async () => {
  suggestAddressesMock.mockResolvedValue({ suggestions: [], total: 2 });
  renderPage();

  const button = await screen.findByRole('button', { name: 'Suggest addresses' });
  fireEvent.click(button);

  expect(
    await screen.findByText('2 address suggestions found — review them below.'),
  ).toBeInTheDocument();
  expect(suggestAddressesMock).toHaveBeenCalledTimes(1);
});

test('suggesting addresses reports when nothing was found', async () => {
  suggestAddressesMock.mockResolvedValue({ suggestions: [], total: 0 });
  renderPage();

  const button = await screen.findByRole('button', { name: 'Suggest addresses' });
  fireEvent.click(button);

  expect(await screen.findByText('No address suggestions right now.')).toBeInTheDocument();
});

test('surfaces an error when suggesting addresses fails', async () => {
  suggestAddressesMock.mockRejectedValue(new Error('network down'));
  renderPage();

  const button = await screen.findByRole('button', { name: 'Suggest addresses' });
  fireEvent.click(button);

  expect(await screen.findByText('Could not scan for address suggestions.')).toBeInTheDocument();
});

test('downloading the CSV export shows a success message', async () => {
  exportCsvMock.mockResolvedValue(undefined);
  renderPage();

  const button = await screen.findByRole('button', { name: 'Download CSV' });
  fireEvent.click(button);

  expect(await screen.findByText('Your data has been exported successfully.')).toBeInTheDocument();
  expect(exportCsvMock).toHaveBeenCalledTimes(1);
});

test('surfaces the CSV export error message from a thrown Error', async () => {
  exportCsvMock.mockRejectedValue(new Error('disk full'));
  renderPage();

  const button = await screen.findByRole('button', { name: 'Download CSV' });
  fireEvent.click(button);

  expect(await screen.findByText('disk full')).toBeInTheDocument();
});

test('falls back to a generic CSV export error for a non-Error rejection', async () => {
  exportCsvMock.mockRejectedValue('some non-error value');
  renderPage();

  const button = await screen.findByRole('button', { name: 'Download CSV' });
  fireEvent.click(button);

  expect(await screen.findByText('Failed to export data. Please try again.')).toBeInTheDocument();
});

test('downloading the VCF export shows a success message', async () => {
  exportVcfMock.mockResolvedValue(undefined);
  renderPage();

  const button = await screen.findByRole('button', { name: 'Download VCF' });
  fireEvent.click(button);

  expect(
    await screen.findByText('Your contacts have been exported successfully.'),
  ).toBeInTheDocument();
  expect(exportVcfMock).toHaveBeenCalledTimes(1);
});

test('surfaces the VCF export error message from a thrown Error', async () => {
  exportVcfMock.mockRejectedValue(new Error('timed out'));
  renderPage();

  const button = await screen.findByRole('button', { name: 'Download VCF' });
  fireEvent.click(button);

  expect(await screen.findByText('timed out')).toBeInTheDocument();
});

test('opening the custom export picker and completing an export shows a success message', async () => {
  exportCustomMock.mockResolvedValue(undefined);
  renderPage();

  const openButton = await screen.findByRole('button', { name: 'Custom export...' });
  fireEvent.click(openButton);

  const exportButton = await screen.findByRole('button', { name: 'picker-export' });
  fireEvent.click(exportButton);

  expect(
    await screen.findByText('Your contacts have been exported successfully.'),
  ).toBeInTheDocument();
  expect(exportCustomMock).toHaveBeenCalledWith('vcf4', {
    sections: ['name'],
    includeSensitive: false,
  });

  // The dialog closes itself via onClose from within the mock, but the page
  // also owns pickerOpen — verify it can be dismissed independently too.
  fireEvent.click(screen.getByRole('button', { name: 'picker-close' }));
  await waitFor(() =>
    expect(screen.queryByRole('button', { name: 'picker-close' })).not.toBeInTheDocument(),
  );
});

test('surfaces the custom export error message from a thrown Error', async () => {
  exportCustomMock.mockRejectedValue(new Error('bad selection'));
  renderPage();

  fireEvent.click(await screen.findByRole('button', { name: 'Custom export...' }));
  fireEvent.click(await screen.findByRole('button', { name: 'picker-export' }));

  expect(await screen.findByText('bad selection')).toBeInTheDocument();
});

test('opening and closing the file import dialog, then reloading history on completion', async () => {
  renderPage();

  fireEvent.click(await screen.findByRole('button', { name: 'Import Contacts' }));
  expect(await screen.findByRole('button', { name: 'import-close' })).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'import-close' }));
  await waitFor(() =>
    expect(screen.queryByRole('button', { name: 'import-close' })).not.toBeInTheDocument(),
  );

  historyMock.mockClear();
  fireEvent.click(screen.getByRole('button', { name: 'Import Contacts' }));
  fireEvent.click(await screen.findByRole('button', { name: 'import-complete' }));

  await waitFor(() => expect(historyMock).toHaveBeenCalledTimes(1));
});

test('opening and closing the Monica import dialog, then reloading history on completion', async () => {
  renderPage();

  fireEvent.click(await screen.findByRole('button', { name: 'Import from Monica' }));
  expect(await screen.findByRole('button', { name: 'monica-close' })).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'monica-close' }));
  await waitFor(() =>
    expect(screen.queryByRole('button', { name: 'monica-close' })).not.toBeInTheDocument(),
  );

  historyMock.mockClear();
  fireEvent.click(screen.getByRole('button', { name: 'Import from Monica' }));
  fireEvent.click(await screen.findByRole('button', { name: 'monica-complete' }));

  await waitFor(() => expect(historyMock).toHaveBeenCalledTimes(1));
});

test('opening and closing the Meerkat import dialog, then reloading history on completion', async () => {
  renderPage();

  fireEvent.click(await screen.findByRole('button', { name: 'Import from Meerkat' }));
  expect(await screen.findByRole('button', { name: 'meerkat-close' })).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'meerkat-close' }));
  await waitFor(() =>
    expect(screen.queryByRole('button', { name: 'meerkat-close' })).not.toBeInTheDocument(),
  );

  historyMock.mockClear();
  fireEvent.click(screen.getByRole('button', { name: 'Import from Meerkat' }));
  fireEvent.click(await screen.findByRole('button', { name: 'meerkat-complete' }));

  await waitFor(() => expect(historyMock).toHaveBeenCalledTimes(1));
});
