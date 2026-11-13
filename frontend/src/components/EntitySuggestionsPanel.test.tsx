import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  dismissEntitySuggestion,
  type EntitySuggestionPair,
  listEntitySuggestions,
} from '../api/entities';
import { SnackbarProvider } from '../context/SnackbarContext';
import EntitySuggestionsPanel from './EntitySuggestionsPanel';

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, listEntitySuggestions: vi.fn(), dismissEntitySuggestion: vi.fn() };
});
const listMock = vi.mocked(listEntitySuggestions);
const dismissMock = vi.mocked(dismissEntitySuggestion);

const pair: EntitySuggestionPair = {
  a: { id: 'a1', kind: 'organisation', name: 'Acme' },
  b: { id: 'b1', kind: 'organisation', name: 'Acme Ltd' },
  reason: 'normalized_name',
};

function renderPanel(resolved: ReadonlySet<string> = new Set()) {
  const onMerge = vi.fn();
  render(
    <SnackbarProvider>
      <EntitySuggestionsPanel kind="organisation" onMerge={onMerge} resolvedPairKeys={resolved} />
    </SnackbarProvider>,
  );
  return { onMerge };
}

beforeEach(() => {
  listMock.mockReset();
  dismissMock.mockReset();
});
afterEach(cleanup);

// issue #2570: loading, error and empty states of the duplicates panel.
test('nothing is listed before the user asks, and the scan button is disabled while loading', async () => {
  let resolve!: (r: { pairs: EntitySuggestionPair[] }) => void;
  listMock.mockReturnValue(new Promise((r) => (resolve = r)) as never);
  renderPanel();
  expect(screen.queryByText('No duplicates found.')).not.toBeInTheDocument();
  expect(listMock).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Find duplicates' })).toBeDisabled(),
  );
  expect(listMock).toHaveBeenCalledWith({ kind: 'organisation', mode: 'pairs' });

  await act(async () => resolve({ pairs: [pair] }));
  expect(screen.getByRole('button', { name: 'Find duplicates' })).toBeEnabled();
  expect(screen.getByText('Acme and Acme Ltd')).toBeInTheDocument();
});

test('a failed scan shows the error and no result list', async () => {
  listMock.mockRejectedValue(new Error('scan broke'));
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('scan broke');
  expect(screen.queryByText('No duplicates found.')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Find duplicates' })).toBeEnabled();
});

test('an empty scan says so, and a pair the parent already merged is hidden', async () => {
  listMock.mockResolvedValueOnce({ pairs: [] } as never);
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByText('No duplicates found.')).toBeInTheDocument();
  cleanup();

  listMock.mockResolvedValueOnce({ pairs: [pair] } as never);
  renderPanel(new Set(['a1:b1']));
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByText('No duplicates found.')).toBeInTheDocument();
  expect(screen.queryByText('Acme and Acme Ltd')).not.toBeInTheDocument();
});

test('a failed dismiss shows the error and keeps the pair; a successful one removes it', async () => {
  listMock.mockResolvedValue({ pairs: [pair] } as never);
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  await screen.findByText('Acme and Acme Ltd');

  dismissMock.mockRejectedValueOnce(new Error('dismiss broke'));
  fireEvent.click(screen.getByRole('button', { name: 'Not a duplicate' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('dismiss broke');
  expect(screen.getByText('Acme and Acme Ltd')).toBeInTheDocument();

  dismissMock.mockResolvedValueOnce(undefined as never);
  fireEvent.click(screen.getByRole('button', { name: 'Not a duplicate' }));
  await waitFor(() => expect(screen.queryByText('Acme and Acme Ltd')).not.toBeInTheDocument());
  expect(dismissMock).toHaveBeenLastCalledWith('a1', 'b1');
});
