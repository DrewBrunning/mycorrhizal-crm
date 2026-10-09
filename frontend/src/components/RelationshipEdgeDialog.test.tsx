import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { Contact, ContactsResponse } from '../api/contacts';
import { getContacts } from '../api/contacts';
import type { RelationshipEdge } from '../api/relationshipEdges';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import RelationshipEdgeDialog from './RelationshipEdgeDialog';

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderDialog(props: Partial<React.ComponentProps<typeof RelationshipEdgeDialog>> = {}) {
  const defaults: React.ComponentProps<typeof RelationshipEdgeDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    viewedContactUid: 'alice-uid',
    ...props,
  };
  return render(
    <MemoryRouter>
      <DateFormatProvider>
        <SnackbarProvider>
          <RelationshipEdgeDialog {...defaults} />
        </SnackbarProvider>
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

function edgeViewedAsTarget(): RelationshipEdge {
  // Alice (viewed) is Bob's child: source=bob, type=parent_of, target=alice.
  return {
    id: 'edge-1',
    source_id: 'bob-uid',
    target_id: 'alice-uid',
    type: 'parent_of',
    directional: true,
    source: 'user-confirmed',
    confidence: 1.0,
    status: 'confirmed',
    sensitivity: 'normal',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

function bobContact(): Contact {
  return { ID: 2, uid: 'bob-uid', firstname: 'Bob', lastname: 'Brown' };
}

test('edit mode renders the other party as read-only text, not an editable field', () => {
  renderDialog({ edge: edgeViewedAsTarget(), otherPartyContact: bobContact() });

  expect(screen.getByText('Bob Brown')).toBeInTheDocument();
  // The manual-entry Name text field (from create mode) must not be present.
  expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
  // The entry-mode radio toggle (manual vs linked) is also create-mode only.
  expect(
    screen.queryByText('How would you like to add this relationship?'),
  ).not.toBeInTheDocument();
});

test('edit mode submits source_id/target_id verbatim, never source_thin/target_thin', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ edge: edgeViewedAsTarget(), otherPartyContact: bobContact(), onSave });

  const saveButton = screen.getByRole('button', { name: 'Save' });
  saveButton.click();

  // Wait a tick for the async handleSave to run.
  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());

  const submitted = onSave.mock.calls[0][0];
  expect(submitted.source_id).toBe('bob-uid');
  expect(submitted.target_id).toBe('alice-uid');
  expect(submitted.source_thin).toBeUndefined();
  expect(submitted.target_thin).toBeUndefined();
});

test('create mode shows the entry-mode toggle and manual name field', () => {
  vi.mocked(getContacts).mockResolvedValue({ contacts: [], next_cursor: '', limit: 100 });
  renderDialog();

  expect(screen.getByText('How would you like to add this relationship?')).toBeInTheDocument();
  // MUI appends " *" to a required field's accessible label text.
  expect(screen.getByLabelText('Name *')).toBeInTheDocument();
});

test('linked mode shows a loading spinner in the contact search while contacts are fetched', async () => {
  let resolveFetch!: (r: ContactsResponse) => void;
  vi.mocked(getContacts).mockReturnValue(
    new Promise((resolve) => {
      resolveFetch = resolve;
    }),
  );

  renderDialog();
  fireEvent.click(screen.getByLabelText('Link to existing contact'));

  await screen.findByRole('progressbar');

  resolveFetch({ contacts: [bobContact()], next_cursor: '', limit: 100 });
  await vi.waitFor(() => expect(screen.queryByRole('progressbar')).not.toBeInTheDocument());
});

// --- validation error states ------------------------------------------------

test('saving without picking a relationship type shows the required-type error', async () => {
  renderDialog();

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Relationship type is required')).toBeInTheDocument();
});

test('manual entry with a type but no name shows the name-required error', async () => {
  renderDialog();

  fireEvent.mouseDown(screen.getAllByRole('combobox')[0]);
  fireEvent.click(await screen.findByRole('option', { name: 'Parent' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Name is required')).toBeInTheDocument();
});

test('linked mode with no contact selected shows the contact-required error', async () => {
  vi.mocked(getContacts).mockResolvedValue({ contacts: [], next_cursor: '', limit: 100 });
  renderDialog();

  fireEvent.click(screen.getByLabelText('Link to existing contact'));
  // Linked mode adds the contact-search autocomplete as the first combobox;
  // the relationship-type select is the second.
  fireEvent.mouseDown(screen.getAllByRole('combobox')[1]);
  fireEvent.click(await screen.findByRole('option', { name: 'Parent' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Please select a contact')).toBeInTheDocument();
});

test('a rejected save surfaces the error inside the dialog', async () => {
  const onSave = vi.fn().mockImplementation(() => Promise.reject(new Error('backend exploded')));
  renderDialog({ onSave });

  fireEvent.mouseDown(screen.getAllByRole('combobox')[0]);
  fireEvent.click(await screen.findByRole('option', { name: 'Parent' }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Bob' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  // The message appears both as the inline error and the snackbar.
  expect(await screen.findAllByText('backend exploded')).not.toHaveLength(0);
});
