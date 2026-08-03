import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import '../i18n/config';
import ContactHeader from './ContactHeader';
import { ContactRecordResponse } from '../api/contacts';

afterEach(cleanup);

function baseRecord(overrides: Partial<ContactRecordResponse> = {}): ContactRecordResponse {
  return {
    id: 1,
    uid: 'uid-1',
    etag: '',
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
    </ThemeProvider>
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

test('collapses the action buttons into an overflow menu at phone widths (T28)', () => {
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

test('renders the standalone action buttons above phone widths (T28)', () => {
  mockMatchMedia(false);
  renderHeader({ onStayInTouch: vi.fn(), onMergeContact: vi.fn(), onArchiveContact: vi.fn() });

  expect(screen.queryByLabelText('Actions')).not.toBeInTheDocument();
  expect(screen.getByText('Stay in Touch')).toBeInTheDocument();
  expect(screen.getByText('Export vCard')).toBeInTheDocument();
  expect(screen.getByText('Merge')).toBeInTheDocument();
  expect(screen.getByText('Archive')).toBeInTheDocument();
});
