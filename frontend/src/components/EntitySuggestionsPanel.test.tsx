import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  dismissEntitySuggestion,
  type Entity,
  type EntitySuggestionPair,
  getEntity,
  listEntitySuggestions,
} from '../api/entities';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import EntitySuggestionsPanel from './EntitySuggestionsPanel';

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return {
    ...actual,
    listEntitySuggestions: vi.fn(),
    dismissEntitySuggestion: vi.fn(),
    getEntity: vi.fn(),
  };
});
const listMock = vi.mocked(listEntitySuggestions);
const dismissMock = vi.mocked(dismissEntitySuggestion);
const getMock = vi.mocked(getEntity);

const pair: EntitySuggestionPair = {
  a: { id: 'a1', kind: 'organisation', name: 'Acme' },
  b: { id: 'b1', kind: 'organisation', name: 'Acme Ltd' },
  reason: 'normalized_name',
};

function renderPanel(resolved: ReadonlySet<string> = new Set()) {
  const onMerge = vi.fn();
  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EntitySuggestionsPanel kind="organisation" onMerge={onMerge} resolvedPairKeys={resolved} />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
  return { onMerge };
}

beforeEach(() => {
  listMock.mockReset();
  dismissMock.mockReset();
  getMock.mockReset();
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

// #2701: "Green Acres Co-op and Green Acres Co-op" cannot be told apart.
const sameName: EntitySuggestionPair = {
  a: { id: 'g1', kind: 'organisation', name: 'Green Acres Co-op' },
  b: { id: 'g2', kind: 'organisation', name: 'green acres co-op' },
  reason: 'exact_name',
};
const detail = (id: string, created: string, links: number): Entity => ({
  id,
  kind: 'organisation',
  name: 'Green Acres Co-op',
  core: {},
  sensitivity: 'normal',
  revision: 1,
  link_count: links,
  created_at: created,
  updated_at: created,
});

test('a pair with the same name adds each side created date and linked-contact count (#2701)', async () => {
  listMock.mockResolvedValue({ pairs: [sameName] } as never);
  getMock.mockImplementation(async (id) =>
    id === 'g1' ? detail('g1', '2026-03-05T10:00:00Z', 2) : detail('g2', '2026-04-19T10:00:00Z', 1),
  );
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  const pairText = await screen.findByText(/Green Acres Co-op \(added .+, 2 contacts\) and /i);
  expect(pairText.textContent).toMatch(/green acres co-op \(added .+, 1 contact\)$/i);
  expect(getMock).toHaveBeenCalledWith('g1');
  expect(getMock).toHaveBeenCalledWith('g2');
});

test('a pair with different names makes no extra requests and keeps the plain text (#2701)', async () => {
  listMock.mockResolvedValue({ pairs: [pair] } as never);
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByText('Acme and Acme Ltd')).toBeInTheDocument();
  expect(getMock).not.toHaveBeenCalled();
});

test('same-name pair still shows plain names when the detail lookup fails (#2701)', async () => {
  listMock.mockResolvedValue({ pairs: [sameName] } as never);
  getMock.mockRejectedValue(new Error('down'));
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByText('Green Acres Co-op and green acres co-op')).toBeInTheDocument();
});
