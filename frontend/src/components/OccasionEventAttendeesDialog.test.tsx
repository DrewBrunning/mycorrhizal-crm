import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { listCircles } from '../api/circles';
import { getContacts } from '../api/contacts';
import type { OccasionEvent } from '../api/occasionEvents';
import { useOccasionEventAttendees } from '../hooks/useOccasionEvents';
import OccasionEventAttendeesDialog from './OccasionEventAttendeesDialog';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('../hooks/useOccasionEvents', () => ({
  useOccasionEventAttendees: vi.fn(),
}));

vi.mock('../api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/circles')>();
  return { ...actual, listCircles: vi.fn() };
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

const event: OccasionEvent = {
  id: 'ev-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  title: 'Summer BBQ',
  starts_at: '2026-07-04T15:00:00Z',
  sensitivity: 'normal',
};

type HookReturn = ReturnType<typeof useOccasionEventAttendees>;

function makeHookReturn(overrides: Partial<HookReturn> = {}): HookReturn {
  return {
    attendees: [
      {
        id: 'a-1',
        event_id: 'ev-1',
        entity_id: 'uid-1',
        contact_id: 3,
        contact_name: 'Alice',
        rsvp: 'pending',
      },
    ],
    loading: false,
    error: null,
    refresh: vi.fn(),
    handleAdd: vi.fn().mockResolvedValue(undefined),
    handleUpdateRsvp: vi.fn().mockResolvedValue(undefined),
    handleRemove: vi.fn().mockResolvedValue(undefined),
    suggestions: [],
    suggestionsLoading: false,
    loadSuggestions: vi.fn().mockResolvedValue(undefined),
    clearSuggestions: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(useOccasionEventAttendees).mockReturnValue(makeHookReturn());
  vi.mocked(listCircles).mockResolvedValue({
    circles: [
      { id: 'c1', name: 'Friends', created_at: '', updated_at: '' },
      { id: 'c2', name: 'Family', created_at: '', updated_at: '' },
    ],
    total: 2,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(getContacts).mockResolvedValue({
    contacts: [
      { ID: 3, uid: 'uid-1', firstname: 'Alice', lastname: 'Anders' },
      { ID: 5, uid: 'uid-5', firstname: 'Bob', lastname: 'Baker' },
      { ID: 6, uid: 'uid-6', firstname: 'Carol', lastname: 'Cole' },
    ],
    total: 3,
    next_cursor: '',
    limit: 100,
  } as never);
});

test('renders the attendee list and the RSVP hint', async () => {
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);
  expect(screen.getByText('Attendees — Summer BBQ')).toBeInTheDocument();
  expect(screen.getByText('Alice')).toBeInTheDocument();
  expect(
    screen.getByText('Record what each guest told you — no invitation is sent from here.'),
  ).toBeInTheDocument();
  await waitFor(() => expect(listCircles).toHaveBeenCalled());
});

test('shows the empty state when nobody is invited', () => {
  vi.mocked(useOccasionEventAttendees).mockReturnValue(makeHookReturn({ attendees: [] }));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);
  expect(screen.getByText('No one invited yet.')).toBeInTheDocument();
});

test('changing the RSVP calls handleUpdateRsvp', async () => {
  const handleUpdateRsvp = vi.fn().mockResolvedValue(undefined);
  vi.mocked(useOccasionEventAttendees).mockReturnValue(makeHookReturn({ handleUpdateRsvp }));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.mouseDown(screen.getByRole('combobox', { name: /rsvp/i }));
  fireEvent.click(await screen.findByRole('option', { name: 'Declined' }));

  await waitFor(() => expect(handleUpdateRsvp).toHaveBeenCalledWith('uid-1', 'declined'));
});

test('removing an attendee calls handleRemove', async () => {
  const handleRemove = vi.fn().mockResolvedValue(undefined);
  vi.mocked(useOccasionEventAttendees).mockReturnValue(makeHookReturn({ handleRemove }));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
  await waitFor(() => expect(handleRemove).toHaveBeenCalledWith('uid-1'));
});

test('selecting a circle and suggesting calls loadSuggestions', async () => {
  const loadSuggestions = vi.fn().mockResolvedValue(undefined);
  vi.mocked(useOccasionEventAttendees).mockReturnValue(makeHookReturn({ loadSuggestions }));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  // The Suggest button is disabled until a circle is selected.
  expect(screen.getByRole('button', { name: 'Suggest' })).toBeDisabled();

  fireEvent.mouseDown(screen.getByRole('combobox', { name: /circles/i }));
  fireEvent.click(await screen.findByRole('option', { name: 'Friends' }));
  fireEvent.click(screen.getByRole('button', { name: 'Suggest' }));

  await waitFor(() => expect(loadSuggestions).toHaveBeenCalledWith(['c1']));
});

test('adds a suggested invitee', async () => {
  const handleAdd = vi.fn().mockResolvedValue(undefined);
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({
      handleAdd,
      suggestions: [{ contact_id: 5, contact_name: 'Bob', entity_id: 'uid-5' }],
    }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  expect(screen.getByText('Bob')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  await waitFor(() => expect(handleAdd).toHaveBeenCalledWith('uid-5'));
});

test('surfaces an attendee action failure', async () => {
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({ handleRemove: vi.fn().mockRejectedValue(new Error('boom')) }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
  expect(await screen.findByText('Failed to update the attendee.')).toBeInTheDocument();
});

test('surfaces a load error from the hook', () => {
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({ error: 'load boom', attendees: [] }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);
  expect(screen.getByText('load boom')).toBeInTheDocument();
});

// Error paths pinned directly: the per-file coverage ratchet showed these were
// only hit incidentally (by other test files, order-dependently), so a CI run
// that didn't exercise them read as a coverage drop.

test('a successful suggested add clears the suggestion list', async () => {
  const clearSuggestions = vi.fn();
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({
      clearSuggestions,
      suggestions: [{ contact_id: 5, contact_name: 'Bob', entity_id: 'uid-5' }],
    }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);
  clearSuggestions.mockClear(); // the open effect clears once on mount

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  await waitFor(() => expect(clearSuggestions).toHaveBeenCalledTimes(1));
});

test('a failed suggested add surfaces an error and keeps the suggestions', async () => {
  const clearSuggestions = vi.fn();
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({
      clearSuggestions,
      handleAdd: vi.fn().mockRejectedValue(new Error('boom')),
      suggestions: [{ contact_id: 5, contact_name: 'Bob', entity_id: 'uid-5' }],
    }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);
  clearSuggestions.mockClear();

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  expect(await screen.findByText('Failed to update the attendee.')).toBeInTheDocument();
  expect(clearSuggestions).not.toHaveBeenCalled();
});

test('a failed RSVP change surfaces an error', async () => {
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({ handleUpdateRsvp: vi.fn().mockRejectedValue(new Error('boom')) }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.mouseDown(screen.getByRole('combobox', { name: /rsvp/i }));
  fireEvent.click(await screen.findByRole('option', { name: 'Declined' }));
  expect(await screen.findByText('Failed to update the attendee.')).toBeInTheDocument();
});

test('a failed circle load leaves the circle picker empty instead of crashing', async () => {
  vi.mocked(listCircles).mockRejectedValue(new Error('boom'));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  await waitFor(() => expect(listCircles).toHaveBeenCalled());
  fireEvent.mouseDown(screen.getByRole('combobox', { name: /circles/i }));
  expect(screen.queryByRole('option', { name: 'Friends' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Suggest' })).toBeDisabled();
});

// Issue #2178: invitees can be picked from contacts by search, not only via
// circles.

test('searching contacts and picking one adds them as an attendee', async () => {
  const handleAdd = vi.fn().mockResolvedValue(undefined);
  vi.mocked(useOccasionEventAttendees).mockReturnValue(makeHookReturn({ handleAdd }));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  const input = screen.getByRole('combobox', { name: /add contacts/i });
  fireEvent.focus(input);
  fireEvent.mouseDown(input);
  fireEvent.change(input, { target: { value: 'Bob' } });
  // Typing narrows the server search (debounced); picking before it fires
  // would clear the input and cancel it.
  await waitFor(() =>
    expect(getContacts).toHaveBeenCalledWith(expect.objectContaining({ search: 'Bob' })),
  );
  fireEvent.click(await screen.findByRole('option', { name: 'Bob Baker' }));

  await waitFor(() => expect(handleAdd).toHaveBeenCalledWith('uid-5'));
});

test('picking several contacts adds each of them', async () => {
  const handleAdd = vi.fn().mockResolvedValue(undefined);
  vi.mocked(useOccasionEventAttendees).mockReturnValue(makeHookReturn({ handleAdd }));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  const input = screen.getByRole('combobox', { name: /add contacts/i });
  fireEvent.mouseDown(input);
  fireEvent.click(await screen.findByRole('option', { name: 'Bob Baker' }));
  await waitFor(() => expect(handleAdd).toHaveBeenCalledWith('uid-5'));
  fireEvent.mouseDown(input);
  fireEvent.click(await screen.findByRole('option', { name: 'Carol Cole' }));
  await waitFor(() => expect(handleAdd).toHaveBeenCalledWith('uid-6'));
});

test('already-invited contacts are not offered in the contact picker', async () => {
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.mouseDown(screen.getByRole('combobox', { name: /add contacts/i }));
  expect(await screen.findByRole('option', { name: 'Bob Baker' })).toBeInTheDocument();
  expect(screen.queryByRole('option', { name: 'Alice Anders' })).not.toBeInTheDocument();
});

test('a failed contact add surfaces an error', async () => {
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({ handleAdd: vi.fn().mockRejectedValue(new Error('boom')) }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.mouseDown(screen.getByRole('combobox', { name: /add contacts/i }));
  fireEvent.click(await screen.findByRole('option', { name: 'Bob Baker' }));
  expect(await screen.findByText('Failed to update the attendee.')).toBeInTheDocument();
});

test('a failed contact search leaves the picker empty instead of crashing', async () => {
  vi.mocked(getContacts).mockRejectedValue(new Error('boom'));
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  await waitFor(() => expect(getContacts).toHaveBeenCalled());
  fireEvent.mouseDown(screen.getByRole('combobox', { name: /add contacts/i }));
  expect(screen.queryByRole('option', { name: 'Bob Baker' })).not.toBeInTheDocument();
});

test('"Add everyone" adds every suggested member of the chosen circles', async () => {
  const handleAdd = vi.fn().mockResolvedValue(undefined);
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({
      handleAdd,
      suggestions: [
        { contact_id: 5, contact_name: 'Bob', entity_id: 'uid-5' },
        { contact_id: 6, contact_name: 'Carol', entity_id: 'uid-6' },
      ],
    }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.click(screen.getByRole('button', { name: 'Add everyone (2)' }));
  await waitFor(() => expect(handleAdd).toHaveBeenCalledTimes(2));
  expect(handleAdd).toHaveBeenCalledWith('uid-5');
  expect(handleAdd).toHaveBeenCalledWith('uid-6');
});

test('with no circles, shows a hint instead of an empty circle picker', async () => {
  vi.mocked(listCircles).mockResolvedValue({ circles: [], total: 0, next_cursor: '', limit: 100 });
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  expect(
    await screen.findByText(
      'You have no circles yet. Create a circle to invite a group in one step, or search for contacts above.',
    ),
  ).toBeInTheDocument();
  expect(screen.queryByRole('combobox', { name: /^circles/i })).not.toBeInTheDocument();
});

test('"Add everyone" stops and reports an error when an add fails', async () => {
  const handleAdd = vi.fn().mockRejectedValue(new Error('boom'));
  vi.mocked(useOccasionEventAttendees).mockReturnValue(
    makeHookReturn({
      handleAdd,
      suggestions: [
        { contact_id: 5, contact_name: 'Bob', entity_id: 'uid-5' },
        { contact_id: 6, contact_name: 'Carol', entity_id: 'uid-6' },
      ],
    }),
  );
  render(<OccasionEventAttendeesDialog open onClose={vi.fn()} event={event} />);

  fireEvent.click(screen.getByRole('button', { name: 'Add everyone (2)' }));

  expect(await screen.findByText('Failed to update the attendee.')).toBeInTheDocument();
  expect(handleAdd).toHaveBeenCalledTimes(1);
});
