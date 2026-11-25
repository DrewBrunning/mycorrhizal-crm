import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { startMycorrhizalFetch, uploadMycorrhizalBundle } from '../api/mycorrhizalImport';
import { cancelSourceImport, getSourceImportStatus } from '../api/sourceImport';
import { UserFacingError } from '../utils/userFacingError';
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
    cancelSourceImport: vi.fn().mockResolvedValue(undefined),
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
const cancelMock = vi.mocked(cancelSourceImport);
beforeEach(() => {
  uploadMock.mockReset();
  fetchMock.mockClear();
  statusMock.mockClear();
  cancelMock.mockClear();
});

function renderOpen() {
  return render(<MycorrhizalImportDialog open onClose={() => {}} onImportComplete={() => {}} />);
}

function pickFile(name = 'bundle.json') {
  const input = document.getElementById('mycorrhizal-file-input') as HTMLInputElement;
  const file = new File(['{}'], name, { type: 'application/json' });
  fireEvent.change(input, { target: { files: [file] } });
}

function bundleTotals() {
  return {
    contacts: 1,
    relationships: 0,
    notes: 0,
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
  };
}

// Issue #2916: a bundle uploaded but not yet handed to the wizard (Start not
// pressed) used to keep its server session until it expired, and enough
// cancelled tries hit the per-user 429 cap.
test('Cancel after an upload releases the session the wizard never took over', async () => {
  uploadMock.mockResolvedValue({ session_id: 's1', version: 1, totals: bundleTotals() });
  const onClose = vi.fn();
  render(<MycorrhizalImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  pickFile();
  await screen.findByRole('button', { name: 'Start import' });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/import/mycorrhizal', 's1'));
  expect(onClose).toHaveBeenCalledTimes(1);
});

test('Cancel before any upload releases nothing', () => {
  const onClose = vi.fn();
  render(<MycorrhizalImportDialog open onClose={onClose} onImportComplete={() => {}} />);

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(onClose).toHaveBeenCalledTimes(1);
  expect(cancelMock).not.toHaveBeenCalled();
});

// Issue #2916: once the wizard owns the uploaded session (Start pressed), the
// dialog's close path must not cancel it a second time — the wizard's own
// cancel does.
test('Cancel after starting does not double-cancel the wizard-owned session', async () => {
  uploadMock.mockResolvedValue({ session_id: 's1', version: 1, totals: bundleTotals() });
  const onClose = vi.fn();
  render(<MycorrhizalImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  pickFile();
  await screen.findByRole('button', { name: 'Start import' });
  fireEvent.click(screen.getByRole('button', { name: 'Start import' }));
  await screen.findByRole('button', { name: 'Cancel' });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/import/mycorrhizal', 's1'));
  expect(cancelMock).toHaveBeenCalledTimes(1);
  expect(onClose).toHaveBeenCalledTimes(1);
});

// Issue #2916: a failed best-effort cancel must never stop the dialog closing.
test('a rejected cancel is swallowed and the dialog still closes', async () => {
  uploadMock.mockResolvedValue({ session_id: 's3', version: 1, totals: bundleTotals() });
  cancelMock.mockImplementationOnce(() => Promise.reject(new UserFacingError('cancel failed')));
  const onClose = vi.fn();
  render(<MycorrhizalImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  pickFile();
  await screen.findByRole('button', { name: 'Start import' });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/import/mycorrhizal', 's3'));
});

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
    Promise.reject(new UserFacingError('Unsupported account bundle version')),
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

// #2688: a bundle is uploaded from a file, so the stepper reads
// Upload -> Review -> Import -> Done, not Connect/Fetch.
test('the stepper names the file upload, not Connect/Fetch', () => {
  renderOpen();
  const labels = Array.from(document.querySelectorAll('.MuiStepLabel-label')).map(
    (el) => el.textContent,
  );
  expect(labels).toEqual(['Upload', 'Review', 'Import', 'Done']);
});

// Issue #2916 follow-up: Cancel stays enabled while a bundle is uploading. An
// upload that finishes after the dialog closed still opened a server session,
// and nothing was left to cancel it, so it held a slot until it expired.
test('Cancel during an upload releases the session once the upload finishes', async () => {
  let finish!: (v: Awaited<ReturnType<typeof uploadMycorrhizalBundle>>) => void;
  uploadMock.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const onClose = vi.fn();
  render(<MycorrhizalImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  pickFile();
  await screen.findByRole('button', { name: 'Uploading…' });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onClose).toHaveBeenCalledTimes(1);

  finish({ session_id: 'late', version: 1, totals: bundleTotals() });
  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/import/mycorrhizal', 'late'));
  // The late result is dropped, not shown when the dialog opens again.
  expect(screen.queryByRole('button', { name: 'Start import' })).not.toBeInTheDocument();
});

test('an upload that fails after Cancel shows no error', async () => {
  let fail!: (e: Error) => void;
  uploadMock.mockReturnValue(
    new Promise((_, reject) => {
      fail = reject;
    }),
  );
  render(<MycorrhizalImportDialog open onClose={() => {}} onImportComplete={() => {}} />);
  pickFile();
  await screen.findByRole('button', { name: 'Uploading…' });
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  fail(new Error('upload exploded'));
  await new Promise((r) => setTimeout(r, 0));
  expect(screen.queryByText('upload exploded')).not.toBeInTheDocument();
  expect(cancelMock).not.toHaveBeenCalled();
});
