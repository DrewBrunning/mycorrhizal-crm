import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  connectMeerkat,
  type MeerkatConnectResponse,
  startMeerkatFetch,
  uploadMeerkatDatabase,
} from '../api/meerkatImport';
import { cancelSourceImport, getSourceImportStatus } from '../api/sourceImport';
import MeerkatImportDialog from './MeerkatImportDialog';

afterEach(cleanup);

vi.mock('../api/meerkatImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/meerkatImport')>();
  return {
    ...actual,
    uploadMeerkatDatabase: vi.fn(),
    connectMeerkat: vi.fn(),
    startMeerkatFetch: vi.fn().mockResolvedValue(undefined),
  };
});

// The cancel and status calls are observed at the shared source-import layer,
// so a cancel from the wizard and one from the dialog are counted together.
vi.mock('../api/sourceImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/sourceImport')>();
  return {
    ...actual,
    cancelSourceImport: vi.fn().mockResolvedValue(undefined),
    getSourceImportStatus: vi.fn(),
  };
});

const cancelMock = vi.mocked(cancelSourceImport);
const statusMock = vi.mocked(getSourceImportStatus);
const uploadMock = vi.mocked(uploadMeerkatDatabase);
const connectMock = vi.mocked(connectMeerkat);
beforeEach(() => {
  uploadMock.mockReset();
  connectMock.mockReset();
  cancelMock.mockClear();
  // Never resolves: the fetching step stays put without polling a real API.
  statusMock.mockReset().mockReturnValue(new Promise(() => {}));
});

function renderOpen() {
  return render(<MeerkatImportDialog open onClose={() => {}} onImportComplete={() => {}} />);
}

function renderFileMode() {
  const view = renderOpen();
  fireEvent.click(screen.getByRole('tab', { name: 'Upload a file' }));
  return view;
}

function pickFile(name = 'meerkat.db') {
  const input = document.getElementById('meerkat-file-input') as HTMLInputElement;
  const file = new File([new Uint8Array([1, 2, 3])], name);
  fireEvent.change(input, { target: { files: [file] } });
}

test('after upload the multi-user picker renders, pre-selects the default, and can start', async () => {
  uploadMock.mockResolvedValue({
    session_id: 's1',
    default_source_user_id: 1,
    totals: {
      contacts: 5,
      relationships: 2,
      notes: 3,
      activities: 0,
      reminders: 0,
    },
    source_users: [
      { id: 1, username: 'fixture', email: '', name: 'Fixture', contacts: 4 },
      { id: 2, username: 'user2', email: '', name: '', contacts: 1 },
    ],
  });

  renderFileMode();
  pickFile();

  expect(await screen.findByText('5 contacts, 2 relationships, 3 notes.')).toBeInTheDocument();
  const radios = screen.getAllByRole('radio');
  expect(radios).toHaveLength(2);
  expect(radios[0]).toBeChecked(); // default_source_user_id = 1
  expect(screen.getByText('Fixture — 4 contacts')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Start import' })).toBeInTheDocument();
});

test('a single-user database skips the picker', async () => {
  uploadMock.mockResolvedValue({
    session_id: 's1',
    default_source_user_id: 1,
    totals: {
      contacts: 3,
      relationships: 0,
      notes: 0,
      activities: 0,
      reminders: 0,
    },
    source_users: [{ id: 1, username: 'solo', email: '', name: '', contacts: 3 }],
  });
  renderFileMode();
  pickFile();
  await screen.findByText('3 contacts, 0 relationships, 0 notes.');
  expect(screen.queryByRole('radio')).not.toBeInTheDocument();
});

test('a rejected upload shows the error inline and stays on the connect step', async () => {
  uploadMock.mockImplementationOnce(() =>
    Promise.reject(new Error('That file is not a Meerkat database')),
  );
  renderFileMode();
  pickFile('notes.db');

  await waitFor(() =>
    expect(screen.getByText('That file is not a Meerkat database')).toBeInTheDocument(),
  );
  expect(screen.getByRole('button', { name: 'Choose database file' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Start import' })).not.toBeInTheDocument();
});

test('Cancel closes the dialog through the reset path', () => {
  const onClose = vi.fn();
  render(<MeerkatImportDialog open onClose={onClose} onImportComplete={() => {}} />);

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(onClose).toHaveBeenCalledTimes(1);
});

test('Start import kicks off the source fetch with the selected user', async () => {
  const fetchMock = vi.mocked(startMeerkatFetch);
  fetchMock.mockClear();
  uploadMock.mockResolvedValue({
    session_id: 's1',
    default_source_user_id: 1,
    totals: {
      contacts: 5,
      relationships: 0,
      notes: 0,
      activities: 0,
      reminders: 0,
    },
    source_users: [
      { id: 1, username: 'a', email: '', name: 'A', contacts: 4 },
      { id: 2, username: 'b', email: '', name: 'B', contacts: 1 },
    ],
  });
  renderFileMode();
  pickFile();

  await screen.findByText('5 contacts, 0 relationships, 0 notes.');
  fireEvent.click(screen.getByRole('button', { name: 'Start import' }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('s1', 1));
});

test('choosing a different source user updates the radio selection', async () => {
  uploadMock.mockResolvedValue({
    session_id: 's1',
    default_source_user_id: 1,
    totals: {
      contacts: 5,
      relationships: 0,
      notes: 0,
      activities: 0,
      reminders: 0,
    },
    source_users: [
      { id: 1, username: 'a', email: '', name: 'A', contacts: 4 },
      { id: 2, username: 'b', email: '', name: 'B', contacts: 1 },
    ],
  });
  renderFileMode();
  pickFile();

  await screen.findByText('5 contacts, 0 relationships, 0 notes.');
  const radios = screen.getAllByRole('radio');
  fireEvent.click(radios[1]);
  expect(radios[1]).toBeChecked();
  expect(radios[0]).not.toBeChecked();
});

const connected = (over: Partial<MeerkatConnectResponse> = {}): MeerkatConnectResponse => ({
  session_id: 'live1',
  version: { generation: 'v1.5-v1.7', supported: true, reason: '', reason_code: '' },
  source_user: { id: 1, username: 'me', email: '', name: 'Me', contacts: 12 },
  totals: {
    contacts: 12,
    relationships: 0,
    notes: 0,
    activities: 0,
    reminders: 0,
  },
  insecure_transport: false,
  estimated_fetch_seconds: 120,
  ...over,
});

function fillUrl(url = 'https://meerkat.example') {
  fireEvent.change(screen.getByLabelText('Meerkat address'), {
    target: { value: url },
  });
}

test('live mode is the default and Connect needs an address and a token', () => {
  renderOpen();
  const connect = screen.getByRole('button', { name: 'Connect' });
  expect(connect).toBeDisabled();
  fillUrl();
  expect(connect).toBeDisabled();
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 'tok' },
  });
  expect(connect).toBeEnabled();
});

test('connecting with a token sends only the token and shows counts + version', async () => {
  connectMock.mockResolvedValue(connected());
  renderOpen();
  fillUrl('  https://meerkat.example  ');
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 'tok' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  expect(await screen.findByText(/12 contacts/)).toBeInTheDocument();
  expect(connectMock).toHaveBeenCalledWith({
    base_url: 'https://meerkat.example',
    api_token: 'tok',
  });
  expect(screen.getByText(/Meerkat v1\.5-v1\.7/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Start import' })).toBeEnabled();
  // The credential is wiped from the form once the server holds it.
  expect(screen.getByLabelText('API token')).toHaveValue('');
});

test('connecting with a username and password sends them instead of a token', async () => {
  connectMock.mockResolvedValue(connected());
  renderOpen();
  fillUrl();
  fireEvent.click(screen.getByRole('radio', { name: 'Username and password' }));
  expect(screen.queryByLabelText('API token')).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('Username or email'), {
    target: { value: 'me' },
  });
  fireEvent.change(screen.getByLabelText('Password'), {
    target: { value: 'pw' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  await screen.findByRole('button', { name: 'Start import' });
  expect(connectMock).toHaveBeenCalledWith({
    base_url: 'https://meerkat.example',
    username: 'me',
    password: 'pw',
  });
  expect(screen.getByLabelText('Password')).toHaveValue('');
});

test('a failed connect shows the error and offers the file upload instead', async () => {
  connectMock.mockRejectedValue(new Error('Those credentials were rejected'));
  renderOpen();
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 'bad' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  expect(await screen.findByText('Those credentials were rejected')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Start import' })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Upload the database file instead' }));
  expect(screen.getByRole('button', { name: 'Choose database file' })).toBeInTheDocument();
});

test('a plain-http address warns that credentials travel unencrypted', async () => {
  connectMock.mockResolvedValue(connected({ insecure_transport: true }));
  renderOpen();
  fillUrl('http://192.168.1.5:3001');
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 't' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  expect(await screen.findByText(/not encrypted/)).toBeInTheDocument();
});

test('an untested version blocks Start until "try anyway" is ticked, then sends accept_untested', async () => {
  const fetchMock = vi.mocked(startMeerkatFetch);
  fetchMock.mockClear();
  connectMock.mockResolvedValue(
    connected({
      version: {
        generation: 'unknown',
        supported: false,
        reason: 'newer than tested',
        reason_code: '',
      },
    }),
  );
  renderOpen();
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 't' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  expect(await screen.findByText(/newer than tested/)).toBeInTheDocument();
  const start = screen.getByRole('button', { name: 'Start import' });
  expect(start).toBeDisabled();
  fireEvent.click(screen.getByRole('checkbox', { name: /Try anyway/ }));
  expect(start).toBeEnabled();
  fireEvent.click(start);

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('live1', undefined, true));
});

async function connectUntested(reason: string, reason_code: string) {
  connectMock.mockResolvedValue(
    connected({ version: { generation: 'unknown', supported: false, reason, reason_code } }),
  );
  renderOpen();
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 't' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await screen.findByRole('button', { name: 'Start import' });
}

test('a known reason code shows the translated sentence, not the server English', async () => {
  await connectUntested('SERVER ENGLISH', 'meerkat_version_newer_than_tested');
  expect(screen.getByText(/looks newer than the versions/)).toBeInTheDocument();
  expect(screen.queryByText(/SERVER ENGLISH/)).not.toBeInTheDocument();
});

test('the older reason code has its own translated sentence', async () => {
  await connectUntested('SERVER ENGLISH', 'meerkat_version_older_than_supported');
  expect(screen.getByText(/looks older than v1\.5/)).toBeInTheDocument();
  expect(screen.queryByText(/SERVER ENGLISH/)).not.toBeInTheDocument();
});

test('an unknown reason code falls back to the server reason', async () => {
  await connectUntested('SERVER ENGLISH', 'some_future_code');
  expect(screen.getByText(/SERVER ENGLISH/)).toBeInTheDocument();
});

test('a supported version starts the fetch without accept_untested', async () => {
  const fetchMock = vi.mocked(startMeerkatFetch);
  fetchMock.mockClear();
  connectMock.mockResolvedValue(connected());
  renderOpen();
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 't' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  fireEvent.click(await screen.findByRole('button', { name: 'Start import' }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('live1', undefined, false));
});

async function connectLive(over: Partial<MeerkatConnectResponse> = {}) {
  connectMock.mockResolvedValue(connected(over));
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 't' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await screen.findByRole('button', { name: 'Start import' });
}

test('closing after Connect but before Start cancels the unowned live session', async () => {
  const onClose = vi.fn();
  render(<MeerkatImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  await connectLive();

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(onClose).toHaveBeenCalledTimes(1);
  expect(cancelMock).toHaveBeenCalledTimes(1);
  expect(cancelMock).toHaveBeenCalledWith('/contacts/import/meerkat', 'live1');
});

test('closing after an upload but before Start cancels the unowned upload session', async () => {
  uploadMock.mockResolvedValue({
    session_id: 'up1',
    default_source_user_id: 1,
    totals: { contacts: 3, relationships: 0, notes: 0, activities: 0, reminders: 0 },
    source_users: [{ id: 1, username: 'solo', email: '', name: '', contacts: 3 }],
  });
  renderFileMode();
  pickFile();
  await screen.findByText('3 contacts, 0 relationships, 0 notes.');

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(cancelMock).toHaveBeenCalledTimes(1);
  expect(cancelMock).toHaveBeenCalledWith('/contacts/import/meerkat', 'up1');
});

test('closing with no session created cancels nothing', () => {
  renderOpen();
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(cancelMock).not.toHaveBeenCalled();
});

test('closing while a connect is in flight cancels the session that arrives afterwards', async () => {
  let resolveConnect: (r: MeerkatConnectResponse) => void = () => {};
  connectMock.mockReturnValue(
    new Promise((resolve) => {
      resolveConnect = resolve;
    }),
  );
  const onClose = vi.fn();
  render(<MeerkatImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 't' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onClose).toHaveBeenCalledTimes(1);
  expect(cancelMock).not.toHaveBeenCalled();

  resolveConnect(connected());
  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/contacts/import/meerkat', 'live1'));
  expect(cancelMock).toHaveBeenCalledTimes(1);
});

test('a session the wizard already owns is cancelled once, not twice', async () => {
  const onClose = vi.fn();
  render(<MeerkatImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  await connectLive();
  fireEvent.click(screen.getByRole('button', { name: 'Start import' }));
  // Fetching has a live backend session, so Cancel asks to discard first.
  await waitFor(() => expect(statusMock).toHaveBeenCalled());

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Discard' }));

  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  expect(cancelMock).toHaveBeenCalledTimes(1);
  expect(cancelMock).toHaveBeenCalledWith('/contacts/import/meerkat', 'live1');
});
test('the live summary shows only the contact count, not zeroed relationships and notes', async () => {
  renderOpen();
  await connectLive();

  expect(screen.getByText('Found 12 contacts.')).toBeInTheDocument();
  expect(screen.queryByText(/relationships/)).not.toBeInTheDocument();
  expect(screen.queryByText(/notes/)).not.toBeInTheDocument();
});

test('both mode tabs are disabled while a connect is in flight', async () => {
  let resolveConnect: (r: MeerkatConnectResponse) => void = () => {};
  connectMock.mockReturnValue(
    new Promise<MeerkatConnectResponse>((resolve) => {
      resolveConnect = resolve;
    }),
  );
  renderOpen();
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 't' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  await screen.findByRole('button', { name: 'Connecting…' });
  expect(screen.getByRole('tab', { name: 'Upload a file' })).toBeDisabled();
  expect(screen.getByRole('tab', { name: 'Connect to Meerkat' })).toBeDisabled();

  resolveConnect(connected());
  await screen.findByRole('button', { name: 'Start import' });
  // A session now exists, so the other tab stays locked.
  expect(screen.getByRole('tab', { name: 'Upload a file' })).toBeDisabled();
});

test('a failed connect re-enables the mode tabs', async () => {
  connectMock.mockRejectedValue(new Error('nope'));
  renderOpen();
  fillUrl();
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 't' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  await screen.findByText('nope');
  expect(screen.getByRole('tab', { name: 'Upload a file' })).toBeEnabled();
});
