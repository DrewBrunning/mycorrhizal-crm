import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { uploadMeerkatDatabase } from '../api/meerkatImport';
import MeerkatImportDialog from './MeerkatImportDialog';

afterEach(cleanup);

vi.mock('../api/meerkatImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/meerkatImport')>();
  return {
    ...actual,
    uploadMeerkatDatabase: vi.fn(),
    startMeerkatFetch: vi.fn().mockResolvedValue(undefined),
  };
});

const uploadMock = vi.mocked(uploadMeerkatDatabase);
beforeEach(() => uploadMock.mockReset());

function renderOpen() {
  return render(<MeerkatImportDialog open onClose={() => {}} onImportComplete={() => {}} />);
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
    totals: { contacts: 5, relationships: 2, notes: 3, activities: 0, reminders: 0 },
    source_users: [
      { id: 1, username: 'fixture', email: '', name: 'Fixture', contacts: 4 },
      { id: 2, username: 'user2', email: '', name: '', contacts: 1 },
    ],
  });

  renderOpen();
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
    totals: { contacts: 3, relationships: 0, notes: 0, activities: 0, reminders: 0 },
    source_users: [{ id: 1, username: 'solo', email: '', name: '', contacts: 3 }],
  });
  renderOpen();
  pickFile();
  await screen.findByText('3 contacts, 0 relationships, 0 notes.');
  expect(screen.queryByRole('radio')).not.toBeInTheDocument();
});

test('a rejected upload shows the error inline and stays on the connect step', async () => {
  uploadMock.mockImplementationOnce(() =>
    Promise.reject(new Error('That file is not a Meerkat database')),
  );
  renderOpen();
  pickFile('notes.db');

  await waitFor(() =>
    expect(screen.getByText('That file is not a Meerkat database')).toBeInTheDocument(),
  );
  expect(screen.getByRole('button', { name: 'Choose database file' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Start import' })).not.toBeInTheDocument();
});
