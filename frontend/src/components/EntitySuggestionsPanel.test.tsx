import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  dismissEntitySuggestion,
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
  a: {
    id: 'a1',
    kind: 'organisation',
    name: 'Acme',
    updated_at: '2026-01-01T10:00:00Z',
    link_count: 0,
  },
  b: {
    id: 'b1',
    kind: 'organisation',
    name: 'Acme Ltd',
    updated_at: '2026-01-02T10:00:00Z',
    link_count: 4,
  },
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

// #2701/#2743: "Green Acres Co-op and Green Acres Co-op" cannot be told apart. Each
// side of a pair now carries updated_at and link_count, so no detail call is needed.
const sameName: EntitySuggestionPair = {
  a: {
    id: 'g1',
    kind: 'organisation',
    name: 'Green Acres Co-op',
    updated_at: '2026-03-05T10:00:00Z',
    link_count: 2,
  },
  b: {
    id: 'g2',
    kind: 'organisation',
    name: 'green acres co-op',
    updated_at: '2026-04-19T10:00:00Z',
    link_count: 1,
  },
  reason: 'exact_name',
};

test('a pair with the same name adds each side updated date and linked-contact count from the pair itself (#2743)', async () => {
  listMock.mockResolvedValue({ pairs: [sameName] } as never);
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  const pairText = await screen.findByText(/Green Acres Co-op \(updated .+, 2 contacts\) and /i);
  expect(pairText.textContent).toMatch(/green acres co-op \(updated .+, 1 contact\)$/i);
  expect(getMock).not.toHaveBeenCalled();
});

test('a pair with different names keeps the plain text and makes no detail requests (#2743)', async () => {
  listMock.mockResolvedValue({ pairs: [pair] } as never);
  renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByText('Acme and Acme Ltd')).toBeInTheDocument();
  expect(getMock).not.toHaveBeenCalled();
});

test('merge is offered the pair with both counts so the dialog need not re-fetch them (#2743)', async () => {
  listMock.mockResolvedValue({ pairs: [sameName] } as never);
  const { onMerge } = renderPanel();
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Merge…' }));
  expect(onMerge).toHaveBeenCalledWith(sameName);
});
