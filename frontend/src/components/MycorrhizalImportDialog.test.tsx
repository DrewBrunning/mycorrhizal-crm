import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { startMycorrhizalFetch, uploadMycorrhizalBundle } from '../api/mycorrhizalImport';
import { getSourceImportStatus } from '../api/sourceImport';
import MycorrhizalImportDialog from './MycorrhizalImportDialog';

afterEach(cleanup);

vi.mock('../api/mycorrhizalImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/mycorrhizalImport')>();
  return {
    ...actual,
    uploadMycorrhizalBundle: vi.fn(),
    startMycorrhizalFetch: vi.fn().mockResolvedValue(undefined),
  };
});

// Starting the fetch kicks off the wizard's status poll. Without this mock the
// poll makes a real request that fails some time after the test has already
// finished, which made the hook's poll-error lines count as covered on some
// runs only (a flaky coverage ratchet).
vi.mock('../api/sourceImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/sourceImport')>();
  return {
    ...actual,
    getSourceImportStatus: vi.fn().mockResolvedValue({
      session_id: 's1',
      phase: 'fetching_contacts',
      phase_done: 0,
      phase_total: 1,
    }),
  };
});

const uploadMock = vi.mocked(uploadMycorrhizalBundle);
const fetchMock = vi.mocked(startMycorrhizalFetch);
const statusMock = vi.mocked(getSourceImportStatus);
beforeEach(() => {
  uploadMock.mockReset();
  fetchMock.mockClear();
  statusMock.mockClear();
});

function renderOpen() {
  return render(<MycorrhizalImportDialog open onClose={() => {}} onImportComplete={() => {}} />);
}

function pickFile(name = 'bundle.json') {
  const input = document.getElementById('mycorrhizal-file-input') as HTMLInputElement;
  const file = new File(['{}'], name, { type: 'application/json' });
  fireEvent.change(input, { target: { files: [file] } });
}

test('after upload the totals render and the import can start', async () => {
  uploadMock.mockResolvedValue({
    session_id: 's1',
    version: 1,
    totals: {
      contacts: 5,
      relationships: 2,
      notes: 3,
      reminders: 0,
      reminder_completions: 0,
      activities: 0,
      life_events: 0,
      gifts: 0,
      preferences: 0,
      conversation_agenda: 0,
      cadence_policies: 0,
      data_decay_policies: 0,
      households: 0,
      circles: 0,
      tags: 0,
      custom_field_definitions: 0,
      custom_field_values: 0,
      occasions: 0,
      occasion_events: 0,
    },
  });

  renderOpen();
  pickFile();

  expect(await screen.findByText('5 contacts, 2 relationships, 3 notes.')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Start import' })).toBeInTheDocument();
});

test('a rejected upload shows the error inline and stays on the connect step', async () => {
  uploadMock.mockImplementationOnce(() =>
    Promise.reject(new Error('Unsupported account bundle version')),
  );
  renderOpen();
  pickFile();

  await waitFor(() =>
    expect(screen.getByText('Unsupported account bundle version')).toBeInTheDocument(),
  );
  expect(screen.getByRole('button', { name: 'Choose bundle file' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Start import' })).not.toBeInTheDocument();
});

test('shows the uploading state, then starts the fetch on Start', async () => {
  let resolveUpload: (v: Awaited<ReturnType<typeof uploadMycorrhizalBundle>>) => void = () => {};
  uploadMock.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveUpload = resolve;
      }),
  );

  renderOpen();
  pickFile();

  expect(await screen.findByRole('button', { name: 'Uploading…' })).toBeInTheDocument();

  resolveUpload({
    session_id: 's1',
    version: 1,
    totals: {
      contacts: 5,
      relationships: 2,
      notes: 3,
      reminders: 0,
      reminder_completions: 0,
      activities: 0,
      life_events: 0,
      gifts: 0,
      preferences: 0,
      conversation_agenda: 0,
      cadence_policies: 0,
      data_decay_policies: 0,
      households: 0,
      circles: 0,
      tags: 0,
      custom_field_definitions: 0,
      custom_field_values: 0,
      occasions: 0,
      occasion_events: 0,
    },
  });

  fireEvent.click(await screen.findByRole('button', { name: 'Start import' }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('s1'));
  await waitFor(() => expect(statusMock).toHaveBeenCalled());
});

test('Escape closes the dialog and resets it', async () => {
  const onClose = vi.fn();
  render(<MycorrhizalImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  fireEvent.keyDown(await screen.findByRole('dialog'), { key: 'Escape' });
  await waitFor(() => expect(onClose).toHaveBeenCalled());
});
