import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import i18n from '../i18n/config';
import '../i18n/config';
import { addCircleMember, type Circle, createCircle } from '../api/circles';
import { createContactRecord } from '../api/contacts';
import { addContactTag, createTag, type Tag } from '../api/tags';
import { resolveEnabledFields } from '../contactFields';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import AddContactDialog from './AddContactDialog';

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, createContactRecord: vi.fn() };
});

vi.mock('../api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/circles')>();
  return { ...actual, createCircle: vi.fn(), addCircleMember: vi.fn() };
});

vi.mock('../api/tags', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/tags')>();
  return { ...actual, createTag: vi.fn(), addContactTag: vi.fn() };
});

afterEach(cleanup);

beforeEach(() => {
  vi.mocked(createContactRecord).mockReset();
  vi.mocked(createCircle).mockReset();
  vi.mocked(addCircleMember).mockReset();
  vi.mocked(createTag).mockReset();
  vi.mocked(addContactTag).mockReset();
});

function renderDialog(
  overrides: Partial<{ availableCircles: Circle[]; availableTags: Tag[] }> = {},
) {
  return render(
    <DateFormatProvider>
      <SnackbarProvider>
        <AddContactDialog
          open
          onClose={vi.fn()}
          onContactAdded={vi.fn()}
          availableCircles={overrides.availableCircles ?? []}
          availableTags={overrides.availableTags ?? []}
        />
      </SnackbarProvider>
    </DateFormatProvider>,
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
    revision: 1,
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
    revision: 1,
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
    revision: 1,
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
    </DateFormatProvider>,
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
    revision: 1,
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
    </DateFormatProvider>,
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

// The alert's dismiss button must carry a localized accessible name -- MUI
// Alert's default "Close" is hardcoded English regardless of app language,
// so this only fails if closeText is wired to a real translation.
test('the error alert dismiss button is localized, not MUI\'s default English "Close"', async () => {
  await i18n.changeLanguage('de');
  try {
    renderDialog();
    fireEvent.click(screen.getByRole('button', { name: 'Erstellen' }));
    await screen.findByRole('alert');
    expect(screen.getByRole('button', { name: 'Schließen' })).toBeInTheDocument();
  } finally {
    await i18n.changeLanguage('en');
  }
});

test('the first-name error clears once the user starts typing a value', async () => {
  renderDialog();

  fireEvent.click(screen.getByRole('button', { name: 'Create' }));
  await screen.findByRole('alert');
  expect(screen.getByLabelText('First Name *')).toHaveAttribute('aria-invalid', 'true');

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Ada' } });

  expect(screen.getByLabelText('First Name *')).toHaveAttribute('aria-invalid', 'false');
});

// #244: under 1.4.12 text-spacing overrides, the Circles section (the last
// form section before DialogActions) can grow tall enough to collide with
// the Cancel button. The scrollable DialogContent needs enough reserved
// bottom padding to survive that worst case.
test('the dialog content reserves extra bottom padding to survive text-spacing growth', () => {
  renderDialog();

  const content = document.querySelector('.MuiDialogContent-root');
  expect(content).not.toBeNull();
  expect(getComputedStyle(content as Element).paddingBottom).toBe('48px');
});

// T52: submitting with only name submits correctly in the simplified flow
test('submits with just first name', async () => {
  const mocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 7,
    uid: 'uid-7',
    etag: '',
    revision: 1,
    card: {},
    crm: {},
  });
  renderDialog();

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Test' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  const components = mocked.mock.calls[0]?.[0].card.name?.components;
  expect(
    components?.some(
      (c: { kind: string; value: string }) => c.kind === 'given' && c.value === 'Test',
    ),
  ).toBe(true);
});

// The "Circles" and "Tags" sections share identical field labels ("Select
// existing .../Or create new...") and button text ("Add"), so every query
// here is scoped to its own section container (the heading's parent Box).
function circlesSection(): HTMLElement {
  return screen.getByText('Circles').parentElement as HTMLElement;
}
function tagsSection(): HTMLElement {
  return screen.getByText('Tags').parentElement as HTMLElement;
}

test('typing a new circle name and clicking Add shows it as a removable chip', () => {
  renderDialog();
  const section = circlesSection();

  fireEvent.change(within(section).getByLabelText('Or create new...'), {
    target: { value: 'Book Club' },
  });
  fireEvent.click(within(section).getByRole('button', { name: 'Add' }));

  expect(within(section).getByText('Book Club')).toBeInTheDocument();

  const chip = within(section).getByText('Book Club').closest('.MuiChip-root') as HTMLElement;
  fireEvent.click(chip.querySelector('svg') as Element);
  expect(within(section).queryByText('Book Club')).not.toBeInTheDocument();
});

test('selecting an existing circle from the dropdown adds it as a chip', () => {
  renderDialog({
    availableCircles: [{ id: 'c1', created_at: '', updated_at: '', name: 'Family' }],
  });
  const section = circlesSection();

  fireEvent.mouseDown(within(section).getByLabelText('Select existing circle...'));
  fireEvent.click(screen.getByRole('option', { name: 'Family' }));

  expect(within(section).getByText('Family')).toBeInTheDocument();
});

test('typing a new tag name and clicking Add shows it as a removable chip', () => {
  renderDialog();
  const section = tagsSection();

  fireEvent.change(within(section).getByLabelText('Or create new...'), {
    target: { value: 'VIP' },
  });
  fireEvent.click(within(section).getByRole('button', { name: 'Add' }));

  expect(within(section).getByText('VIP')).toBeInTheDocument();

  const chip = within(section).getByText('VIP').closest('.MuiChip-root') as HTMLElement;
  fireEvent.click(chip.querySelector('svg') as Element);
  expect(within(section).queryByText('VIP')).not.toBeInTheDocument();
});

test('selecting an existing tag from the dropdown adds it as a chip', () => {
  renderDialog({ availableTags: [{ id: 't1', created_at: '', updated_at: '', name: 'Client' }] });
  const section = tagsSection();

  fireEvent.mouseDown(within(section).getByLabelText('Select existing tag...'));
  fireEvent.click(screen.getByRole('option', { name: 'Client' }));

  expect(within(section).getByText('Client')).toBeInTheDocument();
});

test('creating a contact with a new circle and an existing tag wires up both memberships', async () => {
  const createMocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 9,
    uid: 'uid-9',
    etag: '',
    revision: 1,
    card: {},
    crm: {},
  });
  vi.mocked(createCircle).mockResolvedValue({
    message: 'created',
    circle: { id: 'new-circle-id', created_at: '', updated_at: '', name: 'Book Club' },
  });
  vi.mocked(addCircleMember).mockResolvedValue({
    id: 1,
    circle_id: 'new-circle-id',
    member_vcard_uid: 'uid-9',
  });
  vi.mocked(addContactTag).mockResolvedValue({
    id: 1,
    tag_id: 't1',
    contact_vcard_uid: 'uid-9',
  });

  renderDialog({ availableTags: [{ id: 't1', created_at: '', updated_at: '', name: 'Client' }] });

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Rio' } });
  fireEvent.change(within(circlesSection()).getByLabelText('Or create new...'), {
    target: { value: 'Book Club' },
  });
  fireEvent.click(within(circlesSection()).getByRole('button', { name: 'Add' }));
  fireEvent.mouseDown(within(tagsSection()).getByLabelText('Select existing tag...'));
  fireEvent.click(screen.getByRole('option', { name: 'Client' }));
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(createMocked).toHaveBeenCalled());
  await waitFor(() => expect(createCircle).toHaveBeenCalledWith('Book Club'));
  await waitFor(() => expect(addCircleMember).toHaveBeenCalledWith('new-circle-id', 'uid-9'));
  await waitFor(() => expect(addContactTag).toHaveBeenCalledWith('t1', 'uid-9'));
});

test('a failed circle-membership call is swallowed -- the contact was already created', async () => {
  const createMocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 10,
    uid: 'uid-10',
    etag: '',
    revision: 1,
    card: {},
    crm: {},
  });
  vi.mocked(createCircle).mockRejectedValue(new Error('circle create failed'));
  const onContactAdded = vi.fn();

  render(
    <DateFormatProvider>
      <SnackbarProvider>
        <AddContactDialog
          open
          onClose={vi.fn()}
          onContactAdded={onContactAdded}
          availableCircles={[]}
          availableTags={[]}
        />
      </SnackbarProvider>
    </DateFormatProvider>,
  );

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Sam' } });
  fireEvent.change(within(circlesSection()).getByLabelText('Or create new...'), {
    target: { value: 'Broken Circle' },
  });
  fireEvent.click(within(circlesSection()).getByRole('button', { name: 'Add' }));
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(createMocked).toHaveBeenCalled());
  // Membership failure is silently skipped -- the contact is still reported as added.
  await waitFor(() => expect(onContactAdded).toHaveBeenCalledWith(10));
});

test('shows an error and does not call onContactAdded when contact creation itself fails', async () => {
  vi.mocked(createContactRecord).mockRejectedValue(new Error('server exploded'));
  const onContactAdded = vi.fn();

  render(
    <DateFormatProvider>
      <SnackbarProvider>
        <AddContactDialog
          open
          onClose={vi.fn()}
          onContactAdded={onContactAdded}
          availableCircles={[]}
          availableTags={[]}
        />
      </SnackbarProvider>
    </DateFormatProvider>,
  );

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Fails' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  // May render both as the inline form error and as a snackbar toast.
  await waitFor(() => expect(screen.getAllByText('server exploded').length).toBeGreaterThan(0));
  expect(onContactAdded).not.toHaveBeenCalled();
});
