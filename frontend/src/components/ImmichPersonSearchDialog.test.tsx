import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ImmichPersonSearchDialog from './ImmichPersonSearchDialog';
import { ImmichPerson } from '../api/immich';

afterEach(cleanup);

const people: ImmichPerson[] = [
  { id: 'p-alice', name: 'Alice Example' },
  { id: 'p-bob', name: 'Bob Builder' },
];

function renderDialog(overrides: Partial<React.ComponentProps<typeof ImmichPersonSearchDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ImmichPersonSearchDialog> = {
    open: true,
    onClose: vi.fn(),
    onFetchPeople: vi.fn().mockResolvedValue(people),
    onSelect: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return render(<ImmichPersonSearchDialog {...defaults} />);
}

test('fetches and lists the Immich people on open', async () => {
  renderDialog();
  await waitFor(() => {
    expect(screen.getByText('Alice Example')).toBeInTheDocument();
  });
  expect(screen.getByText('Bob Builder')).toBeInTheDocument();
});

test('search filters the list by name client-side', async () => {
  renderDialog();
  await waitFor(() => expect(screen.getByText('Alice Example')).toBeInTheDocument());

  fireEvent.change(screen.getByLabelText(/Search people by name/), { target: { value: 'bob' } });
  expect(screen.queryByText('Alice Example')).not.toBeInTheDocument();
  expect(screen.getByText('Bob Builder')).toBeInTheDocument();
});

test('picking a person calls onSelect and closes', async () => {
  const onSelect = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onSelect, onClose });

  await waitFor(() => expect(screen.getByText('Alice Example')).toBeInTheDocument());
  fireEvent.click(screen.getByText('Alice Example'));

  await waitFor(() => expect(onSelect).toHaveBeenCalledWith(people[0]));
  expect(onClose).toHaveBeenCalled();
});

test('a fetch failure surfaces the load error', async () => {
  renderDialog({ onFetchPeople: vi.fn().mockRejectedValue(new Error('down')) });
  await waitFor(() => {
    expect(screen.getByText(/Could not load people from Immich/i)).toBeInTheDocument();
  });
});
