import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { connectMonica, startMonicaFetch } from '../api/monicaImport';
import { cancelSourceImport } from '../api/sourceImport';
import MonicaImportDialog from './MonicaImportDialog';

afterEach(cleanup);

vi.mock('../api/monicaImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/monicaImport')>();
  return {
    ...actual,
    connectMonica: vi.fn(),
    startMonicaFetch: vi.fn().mockResolvedValue(undefined),
  };
});

// Cancelling an unowned session goes through the shared source-import layer.
vi.mock('../api/sourceImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/sourceImport')>();
  return {
    ...actual,
    cancelSourceImport: vi.fn().mockResolvedValue(undefined),
  };
});

const connectMock = vi.mocked(connectMonica);
const cancelMock = vi.mocked(cancelSourceImport);

afterEach(() => {
  connectMock.mockReset();
  cancelMock.mockClear();
});

function renderOpen() {
  return render(<MonicaImportDialog open onClose={() => {}} onImportComplete={() => {}} />);
}

test('connects with the entered URL + token and then shows the account summary', async () => {
  connectMock.mockResolvedValue({
    session_id: 'sess-1',
    totals: {
      contacts: 42,
      activities: 8,
      notes: 15,
      reminders: 0,
      calls: 0,
      tasks: 0,
      gifts: 0,
      debts: 0,
    },
    estimated_fetch_seconds: 180,
  });

  renderOpen();

  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok-abc' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  await waitFor(() =>
    expect(connectMock).toHaveBeenCalledWith('https://monica.example', 'tok-abc'),
  );
  expect(
    await screen.findByText('Found 42 contacts, 8 activities and 15 notes.'),
  ).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Start import' })).toBeInTheDocument();
});

test('surfaces a connect failure inline without leaving the connect step', async () => {
  // mockImplementation, not mockRejectedValue: the latter pre-creates the
  // rejected promise (flagged as an unhandled rejection under vitest v4 before
  // handleConnect's await attaches its catch).
  connectMock.mockImplementationOnce(() =>
    Promise.reject(new Error('Monica rejected the API token')),
  );

  renderOpen();
  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'bad' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  expect(await screen.findByText('Monica rejected the API token')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Connect' })).toBeInTheDocument();
});

test('Cancel closes the dialog through the reset path', () => {
  const onClose = vi.fn();
  render(<MonicaImportDialog open onClose={onClose} onImportComplete={() => {}} />);

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(onClose).toHaveBeenCalledTimes(1);
  // Nothing was connected, so there is no server session to release.
  expect(cancelMock).not.toHaveBeenCalled();
});

// Issue #2916: a connected but not-yet-started session used to stay open on
// the server when the dialog closed, counting toward the per-user import cap.
test('Cancel after connecting releases the session the wizard never took over', async () => {
  connectMock.mockResolvedValue({
    session_id: 'sess-1',
    totals: {
      contacts: 1,
      activities: 0,
      notes: 0,
      reminders: 0,
      calls: 0,
      tasks: 0,
      gifts: 0,
      debts: 0,
    },
    estimated_fetch_seconds: 60,
  });
  const onClose = vi.fn();
  render(<MonicaImportDialog open onClose={onClose} onImportComplete={() => {}} />);

  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await screen.findByRole('button', { name: 'Start import' });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/contacts/import/monica', 'sess-1'));
  expect(onClose).toHaveBeenCalledTimes(1);
});

// Issue #2916: once the wizard owns the session (Start pressed), the dialog's
// close path must not cancel it a second time — the wizard's own cancel does.
test('Cancel after starting does not double-cancel the wizard-owned session', async () => {
  connectMock.mockResolvedValue({
    session_id: 'sess-1',
    totals: {
      contacts: 1,
      activities: 0,
      notes: 0,
      reminders: 0,
      calls: 0,
      tasks: 0,
      gifts: 0,
      debts: 0,
    },
    estimated_fetch_seconds: 60,
  });
  const onClose = vi.fn();
  render(<MonicaImportDialog open onClose={onClose} onImportComplete={() => {}} />);

  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await screen.findByRole('button', { name: 'Start import' });
  fireEvent.click(screen.getByRole('button', { name: 'Start import' }));
  await screen.findByRole('button', { name: 'Cancel' });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/contacts/import/monica', 'sess-1'));
  expect(cancelMock).toHaveBeenCalledTimes(1);
  expect(onClose).toHaveBeenCalledTimes(1);
});

// Issue #2916: a failed best-effort cancel must never stop the dialog closing.
test('a rejected cancel is swallowed and the dialog still closes', async () => {
  connectMock.mockResolvedValue({
    session_id: 'sess-2',
    totals: {
      contacts: 1,
      activities: 0,
      notes: 0,
      reminders: 0,
      calls: 0,
      tasks: 0,
      gifts: 0,
      debts: 0,
    },
    estimated_fetch_seconds: 60,
  });
  cancelMock.mockImplementationOnce(() => Promise.reject(new Error('cancel failed')));
  const onClose = vi.fn();
  render(<MonicaImportDialog open onClose={onClose} onImportComplete={() => {}} />);

  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await screen.findByRole('button', { name: 'Start import' });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/contacts/import/monica', 'sess-2'));
});

test('the include-relationships switch toggles off and on', () => {
  renderOpen();

  const toggle = screen.getByRole('switch', {
    name: 'Include relationships between contacts',
  });
  expect(toggle).toBeChecked();

  fireEvent.click(toggle);
  expect(toggle).not.toBeChecked();

  fireEvent.click(toggle);
  expect(toggle).toBeChecked();
});

test('the include-extras switch toggles', () => {
  renderOpen();

  const toggle = screen.getByRole('switch', { name: 'Include calls, tasks, gifts and debts' });
  expect(toggle).toBeChecked();
  fireEvent.click(toggle);
  expect(toggle).not.toBeChecked();
});

test('Start import kicks off the fetch for the connected session', async () => {
  const fetchMock = vi.mocked(startMonicaFetch);
  fetchMock.mockClear();
  connectMock.mockResolvedValue({
    session_id: 'sess-9',
    totals: {
      contacts: 1,
      activities: 0,
      notes: 0,
      reminders: 0,
      calls: 0,
      tasks: 0,
      gifts: 0,
      debts: 0,
    },
    estimated_fetch_seconds: 60,
  });

  renderOpen();
  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  await screen.findByText('Found 1 contact, 0 activities and 0 notes.');
  fireEvent.click(screen.getByRole('button', { name: 'Start import' }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('sess-9', expect.anything()));
});

// #2680: an unreachable Monica reads "Couldn't reach Monica at <host>" with the
// request ref on its own line, not the raw "monica service error" sentence.
test('an unreachable Monica names the host and keeps the ref on a second line', async () => {
  connectMock.mockImplementationOnce(() =>
    Promise.reject(
      new ApiError(
        'monica service error: The Monica instance could not be reached',
        'EXTERNAL_SERVICE_ERROR',
        502,
        undefined,
        '9fe614ba',
        'monica_unreachable',
      ),
    ),
  );
  renderOpen();
  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example/api' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent("Couldn't reach Monica at monica.example");
  expect(alert).toHaveTextContent('(ref: 9fe614ba)');
  expect(alert).not.toHaveTextContent('monica service error');
  expect(screen.getByText('(ref: 9fe614ba)')).not.toBe(alert);
});

// #2680: a URL problem belongs on the address field, not in a banner.
test('an invalid Monica URL is attached to the address field', async () => {
  connectMock.mockImplementationOnce(() =>
    Promise.reject(
      new ApiError(
        'The Monica URL is invalid',
        'VALIDATION_ERROR',
        400,
        undefined,
        undefined,
        'monica_url_invalid',
      ),
    ),
  );
  renderOpen();
  fireEvent.change(screen.getByLabelText('Monica address'), { target: { value: 'nope' } });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  const field = screen.getByLabelText('Monica address');
  await waitFor(() => expect(field).toHaveAttribute('aria-invalid', 'true'));
  expect(field).toHaveAccessibleDescription(/The Monica URL is invalid/);
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();

  // Editing the address clears the error.
  fireEvent.change(field, { target: { value: 'https://monica.example' } });
  expect(field).toHaveAttribute('aria-invalid', 'false');
});

test('other connect failures still use the banner', async () => {
  connectMock.mockImplementationOnce(() =>
    Promise.reject(
      new ApiError(
        'x',
        'EXTERNAL_SERVICE_ERROR',
        502,
        undefined,
        undefined,
        'monica_token_rejected',
      ),
    ),
  );
  renderOpen();
  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'bad' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Monica rejected the API token');
  expect(screen.getByLabelText('Monica address')).toHaveAttribute('aria-invalid', 'false');
});

test('an unreachable Monica with an address that is not a URL names it as typed, without a ref', async () => {
  connectMock.mockImplementationOnce(() =>
    Promise.reject(
      new ApiError('x', 'EXTERNAL_SERVICE_ERROR', 502, undefined, undefined, 'monica_unreachable'),
    ),
  );
  renderOpen();
  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent("Couldn't reach Monica at monica.example");
  expect(alert).not.toHaveTextContent('ref:');
});

// Issue #2916 follow-up: Cancel stays enabled while Connect is in flight. A
// connect that finished after the dialog closed still opened a server session
// that nothing cancelled.
test('Cancel during a connect releases the session once the connect finishes', async () => {
  let finish!: (v: Awaited<ReturnType<typeof connectMonica>>) => void;
  connectMock.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const onClose = vi.fn();
  render(<MonicaImportDialog open onClose={onClose} onImportComplete={() => {}} />);
  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await waitFor(() => expect(connectMock).toHaveBeenCalled());

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onClose).toHaveBeenCalledTimes(1);

  finish({
    session_id: 'late',
    totals: {
      contacts: 1,
      activities: 0,
      notes: 0,
      reminders: 0,
      calls: 0,
      tasks: 0,
      gifts: 0,
      debts: 0,
    },
    estimated_fetch_seconds: 60,
  });
  await waitFor(() => expect(cancelMock).toHaveBeenCalledWith('/contacts/import/monica', 'late'));
  // The late result is dropped, not shown when the dialog opens again.
  expect(screen.queryByRole('button', { name: 'Start import' })).not.toBeInTheDocument();
});

test('a connect that fails after Cancel shows no error', async () => {
  let fail!: (e: Error) => void;
  connectMock.mockReturnValue(
    new Promise((_, reject) => {
      fail = reject;
    }),
  );
  renderOpen();
  fireEvent.change(screen.getByLabelText('Monica address'), {
    target: { value: 'https://monica.example' },
  });
  fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'tok' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await waitFor(() => expect(connectMock).toHaveBeenCalled());
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  fail(new Error('Monica rejected the API token'));
  await new Promise((r) => setTimeout(r, 0));
  expect(screen.queryByText('Monica rejected the API token')).not.toBeInTheDocument();
  expect(cancelMock).not.toHaveBeenCalled();
});
