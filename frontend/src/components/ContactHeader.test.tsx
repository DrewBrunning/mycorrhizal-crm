import { createTheme, ThemeProvider } from '@mui/material/styles';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ContactRecordResponse } from '../api/contacts';
import ContactHeader from './ContactHeader';

afterEach(cleanup);

function baseRecord(overrides: Partial<ContactRecordResponse> = {}): ContactRecordResponse {
  return {
    id: 1,
    uid: 'uid-1',
    etag: '',
    revision: 1,
    card: { name: { components: [{ kind: 'given', value: 'Fluffy' }] } },
    crm: { kind: 'animal' },
    ...overrides,
  };
}

function profileValues(kind: string) {
  return {
    prefix: '',
    firstname: 'Fluffy',
    middle_name: '',
    lastname: '',
    suffix: '',
    nickname: '',
    gender: '',
    kind,
    cardKind: '',
    language: '',
  };
}

const defaultTheme = createTheme();

function renderHeader(props: Partial<React.ComponentProps<typeof ContactHeader>> = {}) {
  const defaults: React.ComponentProps<typeof ContactHeader> = {
    record: baseRecord(),
    profilePic: '',
    editingProfile: false,
    profileValues: profileValues('animal'),
    contactCircles: [],
    contactTags: [],
    allCircles: [],
    allTags: [],
    onStartEditProfile: vi.fn(),
    onCancelEditProfile: vi.fn(),
    onSaveProfile: vi.fn(),
    onDeleteContact: vi.fn(),
    onProfileValueChange: vi.fn(),
    onAddCircle: vi.fn(),
    onRemoveCircle: vi.fn(),
    onAddTag: vi.fn(),
    onRemoveTag: vi.fn(),
    onUploadProfilePicture: vi.fn(),
    onExportContact: vi.fn(),
    ...props,
  };
  return render(
    <ThemeProvider theme={defaultTheme}>
      <ContactHeader {...defaults} />
    </ThemeProvider>,
  );
}

test('view mode labels the contact by its CRM kind (T27)', () => {
  renderHeader();
  expect(screen.getByText('Animal')).toBeInTheDocument();
});

test('view mode does not label a human-kind contact', () => {
  renderHeader({
    record: baseRecord({ crm: { kind: 'human' } }),
    profileValues: profileValues('human'),
  });
  expect(screen.queryByText('Human')).not.toBeInTheDocument();
});

test('edit mode shows the Kind dropdown pre-filled with the contact kind', () => {
  renderHeader({ editingProfile: true });
  expect(screen.getByLabelText('Kind')).toBeInTheDocument();
});

test('changing the Kind in edit mode reports the new value upward', async () => {
  const onProfileValueChange = vi.fn();
  renderHeader({ editingProfile: true, onProfileValueChange });

  fireEvent.mouseDown(screen.getByLabelText('Kind'));
  fireEvent.click(await screen.findByText('Human'));

  expect(onProfileValueChange).toHaveBeenCalledWith(expect.objectContaining({ kind: 'human' }));
});

// MUI's useMediaQuery reads window.matchMedia; jsdom provides none, so give
// the component a controllable implementation per test (T28).
function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

test('collapses the action buttons into an overflow menu below the md breakpoint (T28)', () => {
  mockMatchMedia(true);
  renderHeader({ onStayInTouch: vi.fn(), onMergeContact: vi.fn(), onArchiveContact: vi.fn() });

  expect(screen.getByLabelText('Actions')).toBeInTheDocument();
  expect(screen.queryByText('Stay in Touch')).not.toBeInTheDocument();
  expect(screen.queryByText('Export vCard')).not.toBeInTheDocument();

  fireEvent.click(screen.getByLabelText('Actions'));
  expect(screen.getByText('Stay in Touch')).toBeInTheDocument();
  expect(screen.getByText('Merge')).toBeInTheDocument();
  expect(screen.getByText('Archive')).toBeInTheDocument();
  expect(screen.getByText('vCard 4.0')).toBeInTheDocument();
});

test('renders the standalone action buttons at md and above (T28)', () => {
  mockMatchMedia(false);
  renderHeader({ onStayInTouch: vi.fn(), onMergeContact: vi.fn(), onArchiveContact: vi.fn() });

  expect(screen.queryByLabelText('Actions')).not.toBeInTheDocument();
  expect(screen.getByText('Stay in Touch')).toBeInTheDocument();
  expect(screen.getByText('Export vCard')).toBeInTheDocument();
  expect(screen.getByText('Merge')).toBeInTheDocument();
  expect(screen.getByText('Archive')).toBeInTheDocument();
});

// --- T90: "You" badge + toggle from the overflow menu ----------------------

test('shows a neutral "You" badge on the caller\'s own contact (T90)', () => {
  renderHeader({ isMe: true });
  expect(screen.getByText('You')).toBeInTheDocument();
});

test('shows no "You" badge on other contacts (T90)', () => {
  renderHeader({ isMe: false });
  expect(screen.queryByText('You')).not.toBeInTheDocument();
});

test('the compact overflow menu offers "This is me" and reports the toggle (T90)', () => {
  mockMatchMedia(true);
  const onToggleMe = vi.fn();
  renderHeader({ isMe: false, onToggleMe });

  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByText('This is me'));

  expect(onToggleMe).toHaveBeenCalledTimes(1);
});

test('the overflow menu reads "This isn\'t me" on the current self contact (T90)', () => {
  mockMatchMedia(true);
  renderHeader({ isMe: true, onToggleMe: vi.fn() });

  fireEvent.click(screen.getByLabelText('Actions'));
  expect(screen.getByText("This isn't me")).toBeInTheDocument();
  expect(screen.queryByText('This is me')).not.toBeInTheDocument();
});

// --- Issue #173: favorite star toggle --------------------------------------

test('renders an outline star on a non-favorite and reports the toggle', () => {
  mockMatchMedia(false);
  const onToggleFavorite = vi.fn();
  renderHeader({ onToggleFavorite });

  const star = screen.getByLabelText('Mark as favorite');
  expect(star).toBeInTheDocument();
  fireEvent.click(star);
  expect(onToggleFavorite).toHaveBeenCalledTimes(1);
});

test('renders a filled star on a favorite and reports the toggle', () => {
  mockMatchMedia(false);
  const onToggleFavorite = vi.fn();
  renderHeader({ record: baseRecord({ is_favorite: true }), onToggleFavorite });

  const star = screen.getByLabelText('Unmark as favorite');
  expect(star).toBeInTheDocument();
  fireEvent.click(star);
  expect(onToggleFavorite).toHaveBeenCalledTimes(1);
});

test('renders the star in the compact layout too', () => {
  mockMatchMedia(true);
  renderHeader({ record: baseRecord({ is_favorite: true }), onToggleFavorite: vi.fn() });

  expect(screen.getByLabelText('Unmark as favorite')).toBeInTheDocument();
});

test('renders no star when onToggleFavorite is not provided', () => {
  mockMatchMedia(false);
  renderHeader();
  expect(screen.queryByLabelText('Mark as favorite')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Unmark as favorite')).not.toBeInTheDocument();
});

// --- circle/tag add/remove/create-new flows ---------------------------------

function circle(id: string, name: string) {
  return { id, name, created_at: '', updated_at: '' };
}
function tag(id: string, name: string) {
  return { id, name, created_at: '', updated_at: '' };
}

test('picking an existing circle from the autocomplete calls onAddCircle', async () => {
  mockMatchMedia(false);
  const onAddCircle = vi.fn();
  renderHeader({ allCircles: [circle('c1', 'Friends')], onAddCircle });

  // The first "Edit" pencil belongs to the name section; the circles pencil
  // is the second.
  fireEvent.click(screen.getAllByLabelText('Edit')[1]);
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Select existing circle...' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Friends' }));

  expect(onAddCircle).toHaveBeenCalledWith(circle('c1', 'Friends'));
});

test('pressing Enter in the new-circle field calls onAddCircle with a fresh circle', () => {
  mockMatchMedia(false);
  const onAddCircle = vi.fn();
  renderHeader({ onAddCircle });

  fireEvent.click(screen.getAllByLabelText('Edit')[1]);
  const input = screen.getByPlaceholderText('New circle name...');
  fireEvent.change(input, { target: { value: 'Book Club' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  expect(onAddCircle).toHaveBeenCalledWith({
    id: '',
    created_at: '',
    updated_at: '',
    name: 'Book Club',
  });
  // The field clears after submitting.
  expect(input).toHaveValue('');
});

test('pressing Enter with only whitespace does not call onAddCircle', () => {
  mockMatchMedia(false);
  const onAddCircle = vi.fn();
  renderHeader({ onAddCircle });

  fireEvent.click(screen.getAllByLabelText('Edit')[1]);
  const input = screen.getByPlaceholderText('New circle name...');
  fireEvent.change(input, { target: { value: '   ' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  expect(onAddCircle).not.toHaveBeenCalled();
});

test('deleting a circle chip calls onRemoveCircle', () => {
  mockMatchMedia(false);
  const onRemoveCircle = vi.fn();
  renderHeader({ contactCircles: [circle('c1', 'Friends')], onRemoveCircle });

  fireEvent.click(screen.getAllByLabelText('Edit')[1]);
  // MUI Chip's delete icon renders as a button-like SVG with no accessible
  // name of its own; select it via the chip's delete test id.
  fireEvent.click(screen.getByTestId('CancelIcon'));

  expect(onRemoveCircle).toHaveBeenCalledWith(circle('c1', 'Friends'));
});

test('picking an existing tag from the autocomplete calls onAddTag', async () => {
  mockMatchMedia(false);
  const onAddTag = vi.fn();
  renderHeader({ allTags: [tag('t1', 'VIP')], onAddTag });

  // Edit pencils in order: name, circles, tags.
  fireEvent.click(screen.getAllByLabelText('Edit')[2]);
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Select tag...' }));
  fireEvent.click(await screen.findByRole('option', { name: 'VIP' }));

  expect(onAddTag).toHaveBeenCalledWith(tag('t1', 'VIP'));
});

test('pressing Enter in the new-tag field calls onAddTag with a fresh tag', () => {
  mockMatchMedia(false);
  const onAddTag = vi.fn();
  renderHeader({ onAddTag });

  fireEvent.click(screen.getAllByLabelText('Edit')[2]);
  const input = screen.getByPlaceholderText('New tag...');
  fireEvent.change(input, { target: { value: 'Neighbor' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  expect(onAddTag).toHaveBeenCalledWith({
    id: '',
    created_at: '',
    updated_at: '',
    name: 'Neighbor',
  });
});

test('deleting a tag chip calls onRemoveTag', () => {
  mockMatchMedia(false);
  const onRemoveTag = vi.fn();
  renderHeader({ contactTags: [tag('t1', 'VIP')], onRemoveTag });

  fireEvent.click(screen.getAllByLabelText('Edit')[2]);
  fireEvent.click(screen.getByTestId('CancelIcon'));

  expect(onRemoveTag).toHaveBeenCalledWith(tag('t1', 'VIP'));
});

// --- archived-state button/menu variants ------------------------------------

test('wide layout: an archived contact shows Unarchive and hides Archive/Delete', () => {
  mockMatchMedia(false);
  const onUnarchiveContact = vi.fn();
  renderHeader({
    record: baseRecord({ archived: true }),
    onArchiveContact: vi.fn(),
    onUnarchiveContact,
  });

  expect(screen.getByText('Archived')).toBeInTheDocument();
  const unarchiveButton = screen.getByRole('button', { name: 'Unarchive' });
  expect(unarchiveButton).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Archive' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();

  fireEvent.click(unarchiveButton);
  expect(onUnarchiveContact).toHaveBeenCalledTimes(1);
});

test('wide layout: a non-archived contact shows Archive and Delete, not Unarchive', () => {
  mockMatchMedia(false);
  renderHeader({ onArchiveContact: vi.fn(), onUnarchiveContact: vi.fn() });

  expect(screen.getByRole('button', { name: 'Archive' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Unarchive' })).not.toBeInTheDocument();
});

test("compact layout: an archived contact's overflow menu offers only Unarchive", () => {
  mockMatchMedia(true);
  const onUnarchiveContact = vi.fn();
  renderHeader({
    record: baseRecord({ archived: true }),
    onArchiveContact: vi.fn(),
    onUnarchiveContact,
    onDeleteContact: vi.fn(),
  });

  fireEvent.click(screen.getByLabelText('Actions'));
  expect(screen.getByText('Unarchive')).toBeInTheDocument();
  expect(screen.queryByText('Archive')).not.toBeInTheDocument();
  expect(screen.queryByText('Delete Contact')).not.toBeInTheDocument();

  fireEvent.click(screen.getByText('Unarchive'));
  expect(onUnarchiveContact).toHaveBeenCalledTimes(1);
});
