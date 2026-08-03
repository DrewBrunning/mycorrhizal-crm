import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import AddContactDialog from './AddContactDialog';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import { createContactRecord } from '../api/contacts';

// Mock only the network call; keep every Card/CRM conversion helper real so
// the test asserts on the actual shape the dialog submits.
vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, createContactRecord: vi.fn() };
});

afterEach(cleanup);

beforeEach(() => {
  vi.mocked(createContactRecord).mockReset();
});

function renderDialog() {
  return render(
    <DateFormatProvider>
      <SnackbarProvider>
        <AddContactDialog
          open
          onClose={vi.fn()}
          onContactAdded={vi.fn()}
          availableCircles={[]}
          availableTags={[]}
        />
      </SnackbarProvider>
    </DateFormatProvider>
  );
}

test('shows the Kind dropdown near the top of the form', () => {
  renderDialog();
  // MUI appends " *" only to required labels; Kind is not required.
  expect(screen.getByLabelText('Kind')).toBeInTheDocument();
});

test('defaults the Kind selection to human', async () => {
  const mocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 1,
    uid: 'uid-1',
    etag: '',
    card: {},
    crm: {},
  });
  renderDialog();

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Marie' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  expect(mocked.mock.calls[0][0].crm.kind).toBe('human');
});

test('submits crm.kind = animal when Animal is selected (T27)', async () => {
  const mocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 3,
    uid: 'uid-3',
    etag: '',
    card: {},
    crm: { kind: 'animal' },
  });
  renderDialog();

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Smaug' } });
  fireEvent.mouseDown(screen.getByLabelText('Kind'));
  fireEvent.click(await screen.findByText('Animal'));
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  expect(mocked.mock.calls[0][0].crm.kind).toBe('animal');
});
