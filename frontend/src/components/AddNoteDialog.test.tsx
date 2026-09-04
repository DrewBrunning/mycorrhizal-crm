import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import AddNoteDialog from './AddNoteDialog';

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  // Issue #557: AddNoteDialog persists a draft to sessionStorage while
  // dirty. jsdom's sessionStorage is shared across tests in this file, so a
  // draft left behind by one test would otherwise leak into the next test's
  // fresh render.
  sessionStorage.clear();
  vi.unstubAllGlobals();
});

function mockFetchByUrl(handlers: Record<string, () => unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          return { ok: true, json: async () => respond() };
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

const contactsResponse = () => ({
  contacts: [
    { ID: 5, uid: 'uid-5', firstname: 'Alice', lastname: 'Anderson' },
    { ID: 6, uid: 'uid-6', firstname: 'Bob', lastname: 'Brown' },
  ],
  next_cursor: '',
  limit: 40,
});

function renderDialog(props: Partial<React.ComponentProps<typeof AddNoteDialog>> = {}) {
  const defaults: React.ComponentProps<typeof AddNoteDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(
    <SnackbarProvider>
      <DateFormatProvider>
        <AddNoteDialog {...defaults} />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
}

// T7: AddNoteDialog renders the contact Autocomplete for optional assignment
// at creation time.
test('renders contact autocomplete for assigning a contact at creation', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <AddNoteDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn().mockResolvedValue(undefined)}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  // The contact Autocomplete placeholder is rendered
  await waitFor(() => {
    expect(screen.getByPlaceholderText('Search contacts...')).toBeDefined();
  });

  // Content and date fields exist
  expect(screen.getByLabelText('Content *')).toBeDefined();
  expect(screen.getByLabelText('Date')).toBeDefined();

  // Save and cancel buttons exist
  expect(screen.getByText('Save')).toBeDefined();
  expect(screen.getByText('Cancel')).toBeDefined();
});

// T7b: Dialog title is present.
test('renders the dialog title', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <AddNoteDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn().mockResolvedValue(undefined)}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  await waitFor(() => {
    expect(screen.getByText('Add Note')).toBeDefined();
  });
});

// Issue #557: Cancel on a clean (untouched) form closes immediately -- no
// confirmation for a form with nothing to lose.
test('cancel with no content closes immediately, without a confirmation prompt', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const onClose = vi.fn();
  renderDialog({ onClose });
  await waitFor(() => expect(screen.getByLabelText('Content *')).toBeInTheDocument());

  fireEvent.click(screen.getByText('Cancel'));

  expect(onClose).toHaveBeenCalledTimes(1);
  expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument();
});

// Issue #557 item 3: a dirty note is not silently discarded on Cancel.
test('cancel with typed content asks for confirmation before discarding', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const onClose = vi.fn();
  renderDialog({ onClose });
  await waitFor(() => expect(screen.getByLabelText('Content *')).toBeInTheDocument());

  fireEvent.change(screen.getByLabelText('Content *'), {
    target: { value: 'Called to check in, all is well.' },
  });
  fireEvent.click(screen.getByText('Cancel'));

  expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();

  // "Keep editing" backs out without losing what was typed.
  fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }));
  await waitFor(() =>
    expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument(),
  );
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByLabelText('Content *')).toHaveValue('Called to check in, all is well.');

  // Confirming the discard actually closes.
  fireEvent.click(screen.getByText('Cancel'));
  fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
  expect(onClose).toHaveBeenCalledTimes(1);
});

// Issue #557 item 4: a draft persisted to sessionStorage turns a killed tab
// or a crash-and-retry into an interruption, not a loss. This simulates that
// by unmounting (losing all in-memory React state, the same as a fresh page
// load) and re-rendering a fresh instance of the dialog.
test('a dirty draft survives an unmount/remount (tab close, crash recovery)', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const { unmount } = renderDialog();
  await waitFor(() => expect(screen.getByLabelText('Content *')).toBeInTheDocument());

  fireEvent.change(screen.getByLabelText('Content *'), {
    target: { value: 'Draft note content' },
  });
  await waitFor(() => expect(screen.getByLabelText('Content *')).toHaveValue('Draft note content'));

  unmount();

  renderDialog();
  await waitFor(() => expect(screen.getByLabelText('Content *')).toHaveValue('Draft note content'));
});

// A successful save must not leave a stale draft behind to reappear the next
// time the dialog opens.
test('a successful save clears the draft', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const onSave = vi.fn().mockResolvedValue(undefined);
  const { unmount } = renderDialog({ onSave });
  await waitFor(() => expect(screen.getByLabelText('Content *')).toBeInTheDocument());

  fireEvent.change(screen.getByLabelText('Content *'), { target: { value: 'Saved note' } });
  fireEvent.click(screen.getByText('Save'));
  await waitFor(() => expect(onSave).toHaveBeenCalled());

  unmount();

  renderDialog();
  await waitFor(() => expect(screen.getByLabelText('Content *')).toBeInTheDocument());
  expect(screen.getByLabelText('Content *')).toHaveValue('');
});
