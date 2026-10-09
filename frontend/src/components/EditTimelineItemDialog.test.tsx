import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import EditTimelineItemDialog from './EditTimelineItemDialog';

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
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
  total: 2,
  page: 1,
  limit: 40,
});

// T6: When opened in note mode with an already-assigned contact, the dialog
// shows the contact name in a Chip and the Autocomplete is visible for
// changing the assignment.
test('renders assigned contact name when opened with noteContactId and noteContactName', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn()}
          onDelete={vi.fn()}
          type="note"
          values={{
            noteContent: 'Test note content',
            noteDate: '2026-01-01',
            noteContactId: 5,
            noteContactName: 'Alice Anderson',
          }}
          onChange={vi.fn()}
          allContacts={[]}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  await waitFor(() => {
    expect(screen.getByText('Assigned to Alice Anderson')).toBeDefined();
  });

  // The content and date fields are present
  expect(screen.getByDisplayValue('Test note content')).toBeDefined();
  expect(screen.getByDisplayValue('2026-01-01')).toBeDefined();
});

// T6b: When the contact has no assignment, the dialog shows no Chip and
// the Autocomplete placeholder is visible.
test('shows no contact chip when note has no assignment', async () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn()}
          onDelete={vi.fn()}
          type="note"
          values={{
            noteContent: 'No contact',
            noteDate: '2026-01-01',
          }}
          onChange={vi.fn()}
          allContacts={[]}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  await waitFor(() => {
    expect(screen.queryByText('Assigned to')).toBeNull();
  });

  // The assign-to-contact Autocomplete placeholder is present
  expect(screen.getByPlaceholderText('Search contacts...')).toBeDefined();
});

// The delete action is guarded by window.confirm; both outcomes need pinning
// so a refactor cannot silently make Delete unconditional.
test('delete calls onDelete only when the confirmation is accepted', () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const onDelete = vi.fn();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn()}
          onDelete={onDelete}
          type="note"
          values={{ noteContent: 'Delete me', noteDate: '2026-01-01' }}
          onChange={vi.fn()}
          allContacts={[]}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(confirmSpy).toHaveBeenCalled();
  expect(onDelete).toHaveBeenCalledTimes(1);
  confirmSpy.mockRestore();
});

test('delete does nothing when the confirmation is declined', () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const onDelete = vi.fn();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn()}
          onDelete={onDelete}
          type="note"
          values={{ noteContent: 'Keep me', noteDate: '2026-01-01' }}
          onChange={vi.fn()}
          allContacts={[]}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(confirmSpy).toHaveBeenCalled();
  expect(onDelete).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});

// Activity mode is a wholly separate render branch from note mode.
test('activity mode renders the activity fields and Save calls onSave', () => {
  const onSave = vi.fn();

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open={true}
          onClose={vi.fn()}
          onSave={onSave}
          onDelete={vi.fn()}
          type="activity"
          values={{ activityTitle: 'Coffee', activityLocation: 'Cafe' }}
          onChange={vi.fn()}
          allContacts={[]}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  expect(screen.getByDisplayValue('Coffee')).toBeDefined();
  expect(screen.getByDisplayValue('Cafe')).toBeDefined();
  expect(screen.getByText('Edit Activity')).toBeDefined();

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(onSave).toHaveBeenCalledTimes(1);
});

test('editing the note content and date reports the changes upward', () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const onChange = vi.fn();

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn()}
          onDelete={vi.fn()}
          type="note"
          values={{ noteContent: '', noteDate: '' }}
          onChange={onChange}
          allContacts={[]}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  fireEvent.change(screen.getByLabelText('Content'), { target: { value: 'updated body' } });
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ noteContent: 'updated body' }));

  fireEvent.change(screen.getByLabelText('Date'), { target: { value: '2026-03-03' } });
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ noteDate: '2026-03-03' }));
});

test('clearing the assigned contact removes the assignment', () => {
  mockFetchByUrl({ '/contacts?': contactsResponse });
  const onChange = vi.fn();

  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open={true}
          onClose={vi.fn()}
          onSave={vi.fn()}
          onDelete={vi.fn()}
          type="note"
          values={{
            noteContent: 'x',
            noteDate: '2026-01-01',
            noteContactId: 5,
            noteContactName: 'Alice Anderson',
          }}
          onChange={onChange}
          allContacts={[]}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );

  // The assignment Chip's delete affordance.
  fireEvent.click(screen.getByTestId('CancelIcon'));

  expect(onChange).toHaveBeenCalledWith(
    expect.objectContaining({ noteContactId: undefined, noteContactName: undefined }),
  );
});
