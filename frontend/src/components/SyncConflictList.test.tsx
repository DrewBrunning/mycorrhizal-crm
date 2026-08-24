import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import '../i18n/config';
import SyncConflictList from './SyncConflictList';
import { ContactSyncConflict } from '../api/contactSyncConflicts';

afterEach(cleanup);

function conflict(overrides: Partial<ContactSyncConflict> = {}): ContactSyncConflict {
  return {
    id: 'conflict-1',
    created_at: '2026-08-24T00:00:00Z',
    updated_at: '2026-08-24T00:00:00Z',
    subscription_id: 5,
    contact_id: 42,
    field: 'phone',
    local_value: '[{"type":"work","value":"555-0100"}]',
    remote_value: '[]',
    status: 'pending',
    contact_vcard_uid: 'uid-42',
    contact_name: 'Grace Hopper',
    subscription_name: 'Work address book',
    ...overrides,
  };
}

function renderList(props: Partial<React.ComponentProps<typeof SyncConflictList>> = {}) {
  const defaults: React.ComponentProps<typeof SyncConflictList> = {
    conflicts: [],
    loading: false,
    error: null,
    onRestore: vi.fn(),
    onDismiss: vi.fn(),
    ...props,
  };
  return render(
    <MemoryRouter>
      <SyncConflictList {...defaults} />
    </MemoryRouter>
  );
}

test('renders the conflict naming the field and offering the local value back', () => {
  renderList({ conflicts: [conflict()] });

  expect(screen.getByText('Grace Hopper')).toBeInTheDocument();
  // The notice renders the human-readable local value (array JSON decoded).
  expect(screen.getByText(/555-0100/)).toBeInTheDocument();
  // And a chip naming the field.
  expect(screen.getByText('Phone')).toBeInTheDocument();
  // Row links to the contact.
  const link = screen.getByRole('link');
  expect(link.getAttribute('href')).toBe('/contacts/42');
});

test('restore and dismiss handlers fire with the conflict', () => {
  const onRestore = vi.fn();
  const onDismiss = vi.fn();
  renderList({ conflicts: [conflict()], onRestore, onDismiss });

  fireEvent.click(screen.getByRole('button', { name: 'Restore local value' }));
  expect(onRestore).toHaveBeenCalledWith(conflict());

  fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
  expect(onDismiss).toHaveBeenCalledWith('conflict-1');
});

test('renders scalar values verbatim and the empty state when none exist', () => {
  renderList({ conflicts: [conflict({ field: 'job_title', local_value: 'Local Title', remote_value: 'Remote Title' })] });
  expect(screen.getByText(/Local Title/)).toBeInTheDocument();

  renderList({ conflicts: [] });
  expect(screen.getByText('No sync conflicts right now.')).toBeInTheDocument();
});
