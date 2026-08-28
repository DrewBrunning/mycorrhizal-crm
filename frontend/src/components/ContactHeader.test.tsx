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
