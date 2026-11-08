import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { commitContactMerge, previewContactMerge } from '../api/contactMerge';
import type { Contact } from '../api/contacts';
import BulkMergeDialog from './BulkMergeDialog';

afterEach(cleanup);

vi.mock('../api/contactMerge', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contactMerge')>();
  return {
    ...actual,
    previewContactMerge: vi.fn(),
    commitContactMerge: vi.fn(),
  };
});
vi.mock('../context/SnackbarContext', () => ({
  useSnackbar: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}));

function c(id: number, name: string): Contact {
  return {
    ID: id,
    uid: `uid-${id}`,
    firstname: name,
    lastname: '',
  } as unknown as Contact;
}

const emptyPreview = (keep: number, merge: number) =>
  ({
    keep_id: keep,
    merge_id: merge,
    resolution: {
      emails: [],
      phones: [],
      addresses: [],
      urls: [],
      impps: [],
      resolved_scalars: {},
      conflicts: [],
      field_value_conflicts: [],
    },
    association_counts: {},
  }) as never;

beforeEach(() => {
  vi.mocked(previewContactMerge).mockReset();
  vi.mocked(commitContactMerge).mockReset();
  vi.mocked(previewContactMerge).mockImplementation(async (k, m) => emptyPreview(k, m));
  vi.mocked(commitContactMerge).mockResolvedValue({
    message: 'ok',
    contact: {},
  } as never);
});

const trio = [c(1, 'Alice'), c(2, 'Bob'), c(3, 'Carol')];

test('defaults the primary to the first contact and merges the rest into it', async () => {
  const onDone = vi.fn();
  render(<BulkMergeDialog open contacts={trio} onFinish={onDone} />);

  expect(screen.getByRole('radio', { name: 'Alice' })).toBeChecked();
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }));

  for (const [loser, label] of [
    [2, 'Merge 1 of 2'],
    [3, 'Merge 2 of 2'],
  ] as const) {
    await screen.findByText(label);
    const btn = within(screen.getByRole('dialog')).getByRole('button', {
      name: 'Merge',
    });
    await waitFor(() => expect(btn).not.toBeDisabled());
    fireEvent.click(btn);
    await waitFor(() => expect(commitContactMerge).toHaveBeenCalledWith(1, loser, {}));
  }
  await waitFor(() => expect(onDone).toHaveBeenCalledWith(2));
});

test('keeps merging into whoever survived when the user swaps the keeper mid-way', async () => {
  render(<BulkMergeDialog open contacts={trio} onFinish={vi.fn()} />);
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }));

  await screen.findByText('Merge 1 of 2');
  fireEvent.click(screen.getByRole('radio', { name: 'Keep Bob' }));
  const btn = within(screen.getByRole('dialog')).getByRole('button', {
    name: 'Merge',
  });
  await waitFor(() => expect(btn).not.toBeDisabled());
  fireEvent.click(btn);
  await waitFor(() => expect(commitContactMerge).toHaveBeenCalledWith(2, 1, {}));

  // Bob survived, so Carol is merged into Bob (default keeper = Bob).
  await screen.findByText('Merge 2 of 2');
  await waitFor(() => expect(previewContactMerge).toHaveBeenLastCalledWith(2, 3));
});

test('cancelling part-way still reports the merges that already happened', async () => {
  const onFinish = vi.fn();
  render(<BulkMergeDialog open contacts={trio} onFinish={onFinish} />);
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  await screen.findByText('Merge 1 of 2');
  const btn = within(screen.getByRole('dialog')).getByRole('button', {
    name: 'Merge',
  });
  await waitFor(() => expect(btn).not.toBeDisabled());
  fireEvent.click(btn);
  await screen.findByText('Merge 2 of 2');
  // The first pair's own close (after its commit) must not end the flow.
  expect(onFinish).not.toHaveBeenCalled();

  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(onFinish).toHaveBeenCalledWith(1));
});

test('cancelling before any merge reports zero', () => {
  const onFinish = vi.fn();
  render(<BulkMergeDialog open contacts={trio} onFinish={onFinish} />);
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onFinish).toHaveBeenCalledWith(0);
});
