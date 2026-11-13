import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { type Entity, type EntityListResponse, listEntities } from '../api/entities';
import EntityPicker, { type EntityChoice } from './EntityPicker';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, listEntities: vi.fn() };
});

const listMock = vi.mocked(listEntities);

function entity(id: string, name: string): Entity {
  return {
    id,
    kind: 'organisation',
    name,
    core: {},
    sensitivity: 'normal',
    revision: 1,
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z',
  };
}

function listResponse(...entities: Entity[]): EntityListResponse {
  return { entities, next_cursor: '', limit: 20 } as EntityListResponse;
}

function deferred<T>() {
  let resolve: (v: T) => void = () => undefined;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

beforeEach(() => {
  listMock.mockReset();
  listMock.mockResolvedValue(listResponse());
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

async function flush(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

function renderPicker(props: Partial<React.ComponentProps<typeof EntityPicker>> = {}) {
  const onChange = vi.fn();
  render(<EntityPicker kind="organisation" value={null} onChange={onChange} {...props} />);
  return { onChange, input: screen.getByRole('combobox') as HTMLInputElement };
}

function type(input: HTMLInputElement, text: string) {
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: text } });
}

test('loads the first page of the chosen kind on mount, with no search term', async () => {
  renderPicker({ kind: 'place' });
  await flush(300);
  expect(listMock).toHaveBeenCalledWith({ kind: 'place', q: undefined, limit: 20 });
});

test('typing is debounced: one search with the final trimmed term, none while the user is still typing', async () => {
  const { input } = renderPicker();
  await flush(300);
  listMock.mockClear();

  type(input, 'Ac');
  await flush(100);
  type(input, '  Acme ');
  await flush(100);
  expect(listMock).not.toHaveBeenCalled();

  await flush(200);
  expect(listMock).toHaveBeenCalledTimes(1);
  expect(listMock).toHaveBeenCalledWith({ kind: 'organisation', q: 'Acme', limit: 20 });
});

test('results are listed and choosing one reports the existing entity', async () => {
  listMock.mockResolvedValue(listResponse(entity('e1', 'Acme Corp'), entity('e2', 'Acme Labs')));
  const { input, onChange } = renderPicker();
  type(input, 'Acme');
  await flush(300);

  fireEvent.click(await screen.findByRole('option', { name: 'Acme Labs' }));
  expect(onChange).toHaveBeenCalledWith({ create: false, entity: entity('e2', 'Acme Labs') });
  expect(input.value).toBe('Acme Labs');
});

test('offers Create for a name no result matches, and choosing it keeps the typed name', async () => {
  listMock.mockResolvedValue(listResponse(entity('e1', 'Acme Corp')));
  const { input, onChange } = renderPicker();
  type(input, 'Acme');
  await flush(300);

  fireEvent.click(await screen.findByRole('option', { name: 'Create "Acme"' }));
  expect(onChange).toHaveBeenCalledWith({ create: true, name: 'Acme' });
  expect(input.value).toBe('Acme');
});

test('no Create option when a result already has that name (case-insensitive) or Create is disabled', async () => {
  listMock.mockResolvedValue(listResponse(entity('e1', 'Acme Corp')));
  const first = renderPicker();
  type(first.input, 'acme corp');
  await flush(300);
  await screen.findByRole('option', { name: 'Acme Corp' });
  expect(screen.queryByRole('option', { name: /^Create/ })).toBeNull();
  cleanup();

  const second = renderPicker({ disableCreate: true });
  type(second.input, 'Brand New');
  await flush(300);
  expect(screen.queryByRole('option', { name: /^Create/ })).toBeNull();
});

test('a slow response for an earlier term cannot replace the results of the current one', async () => {
  const slow = deferred<EntityListResponse>();
  const { input } = renderPicker();
  await flush(300);
  listMock.mockReset();
  listMock.mockReturnValueOnce(slow.promise);
  listMock.mockResolvedValueOnce(listResponse(entity('new', 'Newer Result')));

  type(input, 'A');
  await flush(300); // request for "A" is now in flight and never resolves yet
  type(input, 'Ab');
  await flush(300); // request for "Ab" resolves immediately
  expect(await screen.findByRole('option', { name: 'Newer Result' })).toBeDefined();

  await act(async () => {
    slow.resolve(listResponse(entity('old', 'Stale Result')));
    await slow.promise;
  });
  expect(screen.queryByRole('option', { name: 'Stale Result' })).toBeNull();
  expect(screen.getByRole('option', { name: 'Newer Result' })).toBeDefined();
});

test('a failed search shows no results instead of throwing, and Create remains available', async () => {
  listMock.mockRejectedValue(new Error('network down'));
  const { input } = renderPicker();
  type(input, 'Acme');
  await flush(300);

  expect(await screen.findByRole('option', { name: 'Create "Acme"' })).toBeDefined();
});

test('an existing selection pre-fills the field, and a Create selection shows its caption', () => {
  const chosen: EntityChoice = { create: false, entity: entity('e1', 'Acme Corp') };
  const { input } = renderPicker({ value: chosen });
  expect(input.value).toBe('Acme Corp');
  cleanup();

  render(
    <EntityPicker kind="organisation" value={{ create: true, name: 'Newco' }} onChange={vi.fn()} />,
  );
  expect((screen.getByRole('combobox') as HTMLInputElement).value).toBe('');
});

test('clearing the selection reports null', async () => {
  const onChange = vi.fn();
  render(
    <EntityPicker
      kind="organisation"
      value={{ create: false, entity: entity('e1', 'Acme Corp') }}
      onChange={onChange}
    />,
  );
  fireEvent.mouseOver(screen.getByRole('combobox'));
  fireEvent.click(screen.getByTitle('Clear'));
  expect(onChange).toHaveBeenCalledWith(null);
});
