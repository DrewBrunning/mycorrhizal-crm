import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { createEntity, type Entity, updateEntity } from '../api/entities';
import EntityFormDialog from './EntityFormDialog';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, createEntity: vi.fn(), updateEntity: vi.fn() };
});

const createMock = vi.mocked(createEntity);
const updateMock = vi.mocked(updateEntity);

function entity(overrides: Partial<Entity> = {}): Entity {
  return {
    id: 'e1',
    kind: 'organisation',
    name: 'Acme',
    core: { organization: { name: 'Acme' }, url: 'https://acme.test' },
    sensitivity: 'normal',
    revision: 4,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  createMock.mockResolvedValue(entity());
  updateMock.mockResolvedValue(entity());
});

test('creates an organisation with name, website and sensitivity', async () => {
  const onSaved = vi.fn();
  render(<EntityFormDialog open kind="organisation" onClose={vi.fn()} onSaved={onSaved} />);

  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: '  Acme  ' } });
  fireEvent.change(screen.getByLabelText('Website'), { target: { value: 'https://acme.test' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSaved).toHaveBeenCalled());
  expect(createMock).toHaveBeenCalledWith({
    kind: 'organisation',
    name: 'Acme',
    sensitivity: 'normal',
    core: { organization: { name: 'Acme' }, url: 'https://acme.test' },
  });
});

test('shows kind-specific fields: place address, pet species', () => {
  const { rerender } = render(
    <EntityFormDialog open kind="place" onClose={vi.fn()} onSaved={vi.fn()} />,
  );
  expect(screen.getByLabelText('Street')).toBeInTheDocument();
  expect(screen.getByLabelText('City')).toBeInTheDocument();
  rerender(<EntityFormDialog open kind="pet" onClose={vi.fn()} onSaved={vi.fn()} />);
  expect(screen.getByLabelText('Species')).toBeInTheDocument();
  expect(screen.getByLabelText('Birth')).toBeInTheDocument();
});

test('edit mode prefills, sends If-Match revision and never changes the kind', async () => {
  const onSaved = vi.fn();
  render(<EntityFormDialog open entity={entity()} onClose={vi.fn()} onSaved={onSaved} />);

  expect(screen.getByLabelText('Name *')).toHaveValue('Acme');
  expect(screen.getByLabelText('Website')).toHaveValue('https://acme.test');
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Acme Ltd' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSaved).toHaveBeenCalled());
  expect(updateMock).toHaveBeenCalledWith(
    'e1',
    expect.objectContaining({ kind: 'organisation', name: 'Acme Ltd' }),
    4,
  );
});

test('a blank name is rejected before any request', () => {
  render(<EntityFormDialog open kind="organisation" onClose={vi.fn()} onSaved={vi.fn()} />);
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(screen.getByRole('alert')).toHaveTextContent('Enter a name.');
  expect(createMock).not.toHaveBeenCalled();
});

test('an invalid pet date names the field and sends nothing', () => {
  render(<EntityFormDialog open kind="pet" onClose={vi.fn()} onSaved={vi.fn()} />);
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Rex' } });
  fireEvent.change(screen.getByLabelText('Birth'), { target: { value: '2019-13' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(screen.getByRole('alert')).toHaveTextContent('Birth');
  expect(createMock).not.toHaveBeenCalled();
});

test('a server error is shown and the dialog stays open', async () => {
  createMock.mockRejectedValueOnce(
    new ApiError('Name is too long', 'VALIDATION_ERROR', 400, undefined, undefined, undefined),
  );
  const onSaved = vi.fn();
  render(<EntityFormDialog open kind="organisation" onClose={vi.fn()} onSaved={onSaved} />);
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Acme' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Name is too long');
  expect(onSaved).not.toHaveBeenCalled();
});
