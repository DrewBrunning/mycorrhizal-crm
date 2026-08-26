import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ContactShare } from '../api/contactShares';
import type { ImportPreviewResponse } from '../api/import';
import AcceptContactShareDialog from './AcceptContactShareDialog';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

const share: ContactShare = {
  id: 'share-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  from_user_id: 2,
  to_user_id: 1,
  contact_display_name: 'Alice Smith',
  status: 'pending',
};

const preview: ImportPreviewResponse = {
  session_id: 'sess-1',
  rows: [
    {
      row_index: 0,
      parsed_contact: {},
      validation_errors: [],
      duplicate_match: null,
      suggested_action: 'add',
      merge_diff: null,
      batch_duplicate_of: null,
    },
  ],
  total_rows: 1,
  valid_rows: 1,
  duplicate_count: 0,
  error_count: 0,
};

const duplicatePreview: ImportPreviewResponse = {
  ...preview,
  rows: [
    {
      ...preview.rows[0],
      suggested_action: 'update',
      duplicate_match: {
        existing_contact_id: 10,
        existing_firstname: 'Alice',
        existing_lastname: 'Smith',
        existing_email: 'alice@example.com',
        existing_phone: '',
        match_reason: 'email',
      },
    },
  ],
};

function renderDialog(props: Partial<React.ComponentProps<typeof AcceptContactShareDialog>> = {}) {
  const defaults: React.ComponentProps<typeof AcceptContactShareDialog> = {
    open: true,
    onClose: vi.fn(),
    share,
    onAcceptPreview: vi.fn().mockResolvedValue(preview),
    onConfirm: vi
      .fn()
      .mockResolvedValue({ total_processed: 1, created: 1, updated: 0, skipped: 0, errors: 0 }),
  };
  const merged = { ...defaults, ...props };
  render(<AcceptContactShareDialog {...merged} />);
  return {
    onClose: merged.onClose,
    onAcceptPreview: merged.onAcceptPreview,
    onConfirm: merged.onConfirm,
  };
}

test('does not load a preview or render content while closed', () => {
  const { onAcceptPreview } = renderDialog({ open: false });

  expect(onAcceptPreview).not.toHaveBeenCalled();
  expect(screen.queryByText('Review shared contact')).not.toBeInTheDocument();
});

test('loads the preview on open and confirms with the add action', async () => {
  const { onAcceptPreview, onConfirm, onClose } = renderDialog();

  await waitFor(() => expect(onAcceptPreview).toHaveBeenCalledWith('share-1'));
  expect(screen.getByText('Review shared contact')).toBeInTheDocument();
  expect(screen.getByText(/Choose what to do with this shared contact/)).toBeInTheDocument();
  expect(screen.getByText('Add as new')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

  await waitFor(() =>
    expect(onConfirm).toHaveBeenCalledWith('share-1', 'sess-1', [{ row_index: 0, action: 'add' }]),
  );
  await waitFor(() => expect(onClose).toHaveBeenCalled());
});

test('a duplicate match shows the warning chip and preselects the update action', async () => {
  const { onConfirm } = renderDialog({
    onAcceptPreview: vi.fn().mockResolvedValue(duplicatePreview),
  });

  await waitFor(() => expect(screen.getByText('Matches: Alice Smith (email)')).toBeInTheDocument());
  expect(screen.getByText('Update existing')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

  await waitFor(() =>
    expect(onConfirm).toHaveBeenCalledWith('share-1', 'sess-1', [
      { row_index: 0, action: 'update' },
    ]),
  );
});

test('selecting a different action before confirming sends that action', async () => {
  const { onConfirm } = renderDialog({
    onAcceptPreview: vi.fn().mockResolvedValue(duplicatePreview),
  });

  await waitFor(() => expect(screen.getByText('Matches: Alice Smith (email)')).toBeInTheDocument());

  fireEvent.mouseDown(screen.getByLabelText('Action'));
  fireEvent.click(await screen.findByRole('option', { name: 'Skip' }));
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

  await waitFor(() =>
    expect(onConfirm).toHaveBeenCalledWith('share-1', 'sess-1', [{ row_index: 0, action: 'skip' }]),
  );
});

test('a preview failure surfaces the error and disables confirming', async () => {
  renderDialog({
    onAcceptPreview: vi.fn().mockRejectedValue(new Error('preview failed')),
  });

  await waitFor(() => expect(screen.getByText('preview failed')).toBeInTheDocument());
  expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
});

test('a confirm failure surfaces the error and keeps the dialog open', async () => {
  const { onClose } = renderDialog({
    onConfirm: vi.fn().mockRejectedValue(new Error('confirm failed')),
  });

  await waitFor(() => expect(screen.getByLabelText('Action')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

  await waitFor(() => expect(screen.getByText('confirm failed')).toBeInTheDocument());
  expect(onClose).not.toHaveBeenCalled();
});
