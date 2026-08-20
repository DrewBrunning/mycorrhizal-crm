import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import i18n from '../i18n/config';
import '../i18n/config';
import AddContactDialog from './AddContactDialog';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import { createContactRecord } from '../api/contacts';
import { resolveEnabledFields } from '../contactFields';

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

// T52: the simplified add dialog only shows Name + Contact sections
test('Contact section heading is always visible (phones and emails are default-enabled)', () => {
  renderDialog();
  expect(screen.getByText('Contact')).toBeInTheDocument();
});

test('Name section heading is always visible', () => {
  renderDialog();
  expect(screen.getByText('Name')).toBeInTheDocument();
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

test('submits card.language when set', async () => {
  const mocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 4,
    uid: 'uid-4',
    etag: '',
    card: {},
    crm: {},
  });
  render(
    <DateFormatProvider>
      <SnackbarProvider>
        <AddContactDialog
          open
          onClose={vi.fn()}
          onContactAdded={vi.fn()}
          availableCircles={[]}
          availableTags={[]}
          enabledFields={resolveEnabledFields(['language'])}
        />
      </SnackbarProvider>
    </DateFormatProvider>
  );

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Orchestra' } });
  const langInput = screen.getByLabelText('Language');
  fireEvent.change(langInput, { target: { value: 'de' } });
  fireEvent.keyDown(langInput, { key: 'Enter' });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  expect(mocked.mock.calls[0][0].card.language).toBe('de');
});

test('defaults the card language to the UI language when not touched', async () => {
  const mocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 6,
    uid: 'uid-6',
    etag: '',
    card: {},
    crm: {},
  });
  render(
    <DateFormatProvider>
      <SnackbarProvider>
        <AddContactDialog
          open
          onClose={vi.fn()}
          onContactAdded={vi.fn()}
          availableCircles={[]}
          availableTags={[]}
          enabledFields={resolveEnabledFields(['language'])}
        />
      </SnackbarProvider>
    </DateFormatProvider>
  );

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Ada' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  expect(mocked.mock.calls[0][0].card.language).toBe((i18n.language || 'en').split('-')[0]);
});

// #242: the required-field error must reach assistive tech via a live region,
// and the invalid field itself must carry aria-invalid/aria-describedby.
test('an empty first name on submit is announced via role=alert and wires aria-invalid on the field', async () => {
  renderDialog();

  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('First name and last name are required');

  const firstNameField = screen.getByLabelText('First Name *');
  expect(firstNameField).toHaveAttribute('aria-invalid', 'true');
  expect(firstNameField).toHaveAccessibleDescription('First name is required');
});

test('the first-name error clears once the user starts typing a value', async () => {
  renderDialog();

  fireEvent.click(screen.getByRole('button', { name: 'Create' }));
  await screen.findByRole('alert');
  expect(screen.getByLabelText('First Name *')).toHaveAttribute('aria-invalid', 'true');

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Ada' } });

  expect(screen.getByLabelText('First Name *')).toHaveAttribute('aria-invalid', 'false');
});

// T52: submitting with only name submits correctly in the simplified flow
test('submits with just first name', async () => {
  const mocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 7,
    uid: 'uid-7',
    etag: '',
    card: {},
    crm: {},
  });
  renderDialog();

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Test' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  const components = mocked.mock.calls[0]![0].card.name!.components!;
  expect(components.some((c: { kind: string; value: string }) => c.kind === 'given' && c.value === 'Test')).toBe(true);
});
