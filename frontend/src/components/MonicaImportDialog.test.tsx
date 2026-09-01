import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { connectMonica } from '../api/monicaImport';
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

const connectMock = vi.mocked(connectMonica);

afterEach(() => connectMock.mockReset());

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
