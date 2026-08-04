import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor, within } from '@testing-library/react';
import i18n from '../i18n/config';
import '../i18n/config';
import AddContactDialog from './AddContactDialog';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import { createContactRecord } from '../api/contacts';
import { resolveEnabledFields } from '../contactFields';

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

test('submits card.kind = group and card.language when set (WP13/WP4)', async () => {
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
          enabledFields={resolveEnabledFields(['cardKind', 'language'])}
        />
      </SnackbarProvider>
    </DateFormatProvider>
  );

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Orchestra' } });
  fireEvent.mouseDown(screen.getByLabelText('Contact Kind'));
  fireEvent.click(await screen.findByText('Group'));
  // The Language field is a free-solo Autocomplete; typing then pressing Enter
  // commits the typed BCP-47 tag as the card language.
  const langInput = screen.getByLabelText('Language');
  fireEvent.change(langInput, { target: { value: 'de' } });
  fireEvent.keyDown(langInput, { key: 'Enter' });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  expect(mocked.mock.calls[0][0].card.kind).toBe('group');
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

test('submits speakToAs pronouns and personalInfo when filled (WP1/WP2)', async () => {
  const mocked = vi.mocked(createContactRecord).mockResolvedValue({
    id: 5,
    uid: 'uid-5',
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
          enabledFields={resolveEnabledFields(['speakToAs', 'personalInfo'])}
        />
      </SnackbarProvider>
    </DateFormatProvider>
  );

  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Ada' } });
  // Add a pronoun row (SpeakToAs editor) and a personal-info row (PersonalInfo
  // editor), then fill each row's labeled input. Each query is scoped to its
  // editor so the test doesn't depend on the fields' on-screen ordering.
  const speakToAsSection = screen.getByText('Pronouns').parentElement as HTMLElement;
  const personalInfoSection = screen.getByText('Personal Info (Expertise / Hobbies / Interests)').parentElement as HTMLElement;

  fireEvent.click(within(speakToAsSection).getAllByRole('button', { name: 'Add' })[0]);
  fireEvent.change(screen.getByLabelText('Pronouns'), { target: { value: 'she/her' } });
  fireEvent.click(within(personalInfoSection).getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'math' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create' }));

  await waitFor(() => expect(mocked).toHaveBeenCalled());
  expect(mocked.mock.calls[0][0].card.speakToAs?.pronouns?.[0]?.pronouns).toBe('she/her');
  expect(mocked.mock.calls[0][0].card.personalInfo?.[0]?.value).toBe('math');
});
