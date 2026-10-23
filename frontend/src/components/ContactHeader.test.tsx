import { createTheme, ThemeProvider } from '@mui/material/styles';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ContactScoreResponse, ContactScoreTarget } from '../api/contactScore';
import type { ContactRecordResponse } from '../api/contacts';
import { DateFormatProvider } from '../DateFormatProvider';
import { useContactScore } from '../hooks/useContactScore';
import ContactHeader from './ContactHeader';

afterEach(cleanup);

// Issue #383/ADR-0023: ContactHeader fetches the health score itself via
// useContactScore -- mocked here so tests control the score/loading/error
// shape directly, same as other component tests mock their data hooks.
vi.mock('../hooks/useContactScore', () => ({
  useContactScore: vi.fn(),
}));

function scoreFixture(band: string): ContactScoreResponse {
  return {
    contact_id: 1,
    score: 72,
    band,
    recency: {
      value: 80,
      weight: 35,
      reason: 'Last qualifying interaction 5 day(s) ago (target every 14 days)',
      reason_code: 'recency_days',
      reason_params: { days: 5, interval_days: 14 },
    },
    frequency: {
      value: 60,
      weight: 20,
      reason:
        '1 qualifying interaction(s) in the last 180 days (expected ~12.9 at the target cadence)',
      reason_code: 'frequency_count',
      reason_params: { count: 1, window_days: 180, expected: 12.857 },
    },
    closeness: {
      value: 85,
      weight: 20,
      reason: 'Directly connected via a spouse_of relationship',
      reason_code: 'closeness_direct',
      reason_params: { relation: 'spouse_of' },
    },
    reach_out: {
      value: 100,
      weight: 15,
      reason: 'No pending reach-out suggestion',
      reason_code: 'reach_out_none',
      reason_params: {},
    },
    last_updated: {
      value: 90,
      weight: 10,
      reason: 'Contact record last edited 1 day(s) ago',
      reason_code: 'last_updated_days',
      reason_params: { days: 1 },
    },
    target: null,
  };
}

let refreshScoreMock = vi.fn(async () => {});

function mockScore(score: ContactScoreResponse | null) {
  refreshScoreMock = vi.fn(async () => {});
  vi.mocked(useContactScore).mockReturnValue({
    score,
    loading: false,
    error: null,
    refreshScore: refreshScoreMock,
  });
}

function targetFixture(overrides: Partial<ContactScoreTarget> = {}): ContactScoreTarget {
  return {
    tier: 'close',
    status: 'drifting',
    value: 30,
    interval_days: 14,
    interval_source: 'target_tier',
    reason: 'Recency and frequency score 30 against a 14-day interval from the close target',
    reason_code: 'target_tier',
    reason_params: { score: 30, interval_days: 14, tier: 'close' },
    ...overrides,
  };
}

const offer = { entity_id: 'uid-1', target_interval_days: 14 };

beforeEach(() => {
  // Default: no score loaded, matching a fresh contact with no computed
  // score yet -- individual tests override with mockScore(...).
  mockScore(null);
});

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
      <DateFormatProvider>
        <ContactHeader {...defaults} />
      </DateFormatProvider>
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
  expect(screen.getByText('Export as vCard 4.0')).toBeInTheDocument();
});

test('renders the standalone action buttons at md and above (T28)', () => {
  mockMatchMedia(false);
  renderHeader({ onStayInTouch: vi.fn(), onMergeContact: vi.fn(), onArchiveContact: vi.fn() });

  expect(screen.queryByLabelText('Actions')).not.toBeInTheDocument();
  // Issue #1782: three primary actions inline, the rest behind "More".
  expect(screen.getByText('Stay in Touch')).toBeInTheDocument();
  expect(screen.queryByText('Export vCard')).not.toBeInTheDocument();
  expect(screen.queryByText('Merge')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Archive' })).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'More' }));
  expect(screen.getByRole('menuitem', { name: 'Merge' })).toBeInTheDocument();
  expect(screen.getByRole('menuitem', { name: 'Archive' })).toBeInTheDocument();
  // Stay in touch is inline, so it is not duplicated in the menu.
  expect(screen.queryByRole('menuitem', { name: 'Stay in Touch' })).not.toBeInTheDocument();
});

test('the More menu lists Delete last, after a separator (#1782)', () => {
  mockMatchMedia(false);
  renderHeader({ onMergeContact: vi.fn(), onArchiveContact: vi.fn() });

  fireEvent.click(screen.getByRole('button', { name: 'More' }));
  const items = screen.getAllByRole('menuitem');
  expect(items[items.length - 1]).toHaveTextContent('Delete Contact');
  expect(screen.getByRole('separator')).toBeInTheDocument();
});

test('the More button is a focusable menu button (#1782)', () => {
  mockMatchMedia(false);
  renderHeader({ onMergeContact: vi.fn() });

  const more = screen.getByRole('button', { name: 'More' });
  expect(more).toHaveAttribute('aria-haspopup', 'menu');
  more.focus();
  expect(more).toHaveFocus();
  expect(more).not.toHaveAttribute('aria-expanded');
  fireEvent.click(more);
  expect(more).toHaveAttribute('aria-expanded', 'true');
  expect(more.getAttribute('aria-controls')).toBe(screen.getByRole('menu').id);
});

test('compact menu items run their action and close the menu (#1782)', async () => {
  mockMatchMedia(true);
  const onStayInTouch = vi.fn();
  const onExportContact = vi.fn();
  renderHeader({ onStayInTouch, onExportContact });

  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByRole('menuitem', { name: 'Stay in Touch' }));
  expect(onStayInTouch).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());

  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByRole('menuitem', { name: 'Export as vCard 3.0' }));
  expect(onExportContact).toHaveBeenCalledWith('vcf3');
});

test('Escape closes the wide More menu (#1782)', async () => {
  mockMatchMedia(false);
  renderHeader({ onMergeContact: vi.fn() });

  const more = screen.getByRole('button', { name: 'More' });
  fireEvent.click(more);
  fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' });
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());
  expect(more).not.toHaveAttribute('aria-expanded');
});

test('both menu triggers name the menu they open (#1782)', () => {
  mockMatchMedia(true);
  renderHeader({ onMergeContact: vi.fn() });

  const trigger = screen.getByLabelText('Actions');
  expect(trigger).toHaveAttribute('aria-haspopup', 'menu');
  fireEvent.click(trigger);
  expect(trigger).toHaveAttribute('aria-expanded', 'true');
  expect(trigger.getAttribute('aria-controls')).toBe(screen.getByRole('menu').id);
  // Export items carry their action, not just a format name.
  for (const format of ['vCard 4.0', 'vCard 3.0', 'JSContact']) {
    expect(screen.getByRole('menuitem', { name: `Export as ${format}` })).toBeInTheDocument();
  }
});

test('the compact menu also lists Delete last, after a separator (#1782)', () => {
  mockMatchMedia(true);
  renderHeader({ onArchiveContact: vi.fn() });

  fireEvent.click(screen.getByLabelText('Actions'));
  const items = screen.getAllByRole('menuitem');
  expect(items[items.length - 1]).toHaveTextContent('Delete Contact');
  expect(screen.getByRole('separator')).toBeInTheDocument();
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

  fireEvent.click(screen.getByRole('button', { name: 'More' }));
  expect(screen.getByRole('menuitem', { name: 'Archive' })).toBeInTheDocument();
  expect(screen.getByRole('menuitem', { name: 'Delete Contact' })).toBeInTheDocument();
  expect(screen.queryByRole('menuitem', { name: 'Unarchive' })).not.toBeInTheDocument();
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

// --- Issue #1193: deceased badge --------------------------------------------

test('shows no deceased badge for a contact with no death anniversary', () => {
  renderHeader();
  expect(screen.queryByText('Deceased', { exact: false })).not.toBeInTheDocument();
});

test('shows a deceased badge with the date of death for a contact with one', () => {
  renderHeader({
    record: baseRecord({
      card: {
        name: { components: [{ kind: 'given', value: 'Fluffy' }] },
        anniversaries: [{ kind: 'death', date: { partial: { year: 2020, month: 5, day: 1 } } }],
      },
    }),
  });
  expect(screen.getByText('Deceased — 01.05.2020', { exact: false })).toBeInTheDocument();
});

test('shows a bare deceased badge when the death anniversary has no date', () => {
  renderHeader({
    record: baseRecord({
      card: {
        name: { components: [{ kind: 'given', value: 'Fluffy' }] },
        anniversaries: [{ kind: 'death', date: {} }],
      },
    }),
  });
  expect(screen.getByText('Deceased')).toBeInTheDocument();
});

test('shows both the archived and deceased badges together when both apply', () => {
  renderHeader({
    record: baseRecord({
      archived: true,
      card: {
        name: { components: [{ kind: 'given', value: 'Fluffy' }] },
        anniversaries: [{ kind: 'death', date: { partial: { year: 2020, month: 5, day: 1 } } }],
      },
    }),
  });
  expect(screen.getByText('Archived')).toBeInTheDocument();
  expect(screen.getByText('Deceased — 01.05.2020', { exact: false })).toBeInTheDocument();
});

// --- Issue #383/ADR-0023: relationship health score badge + popover -------

test('renders no health badge when no score has loaded', () => {
  mockMatchMedia(false);
  mockScore(null);
  renderHeader();

  expect(screen.queryByText('Healthy')).not.toBeInTheDocument();
  expect(screen.queryByText('Needs Attention')).not.toBeInTheDocument();
  expect(screen.queryByText('Neglected')).not.toBeInTheDocument();
});

test('wide layout: shows a "Healthy" badge for the moss band', () => {
  mockMatchMedia(false);
  mockScore(scoreFixture('moss'));
  renderHeader();

  expect(screen.getByText('Healthy')).toBeInTheDocument();
});

test('wide layout: shows a "Needs Attention" badge for the chanterelle band', () => {
  mockMatchMedia(false);
  mockScore(scoreFixture('chanterelle'));
  renderHeader();

  expect(screen.getByText('Needs Attention')).toBeInTheDocument();
});

test('wide layout: shows a "Neglected" badge for the russula band', () => {
  mockMatchMedia(false);
  mockScore(scoreFixture('russula'));
  renderHeader();

  expect(screen.getByText('Neglected')).toBeInTheDocument();
});

test('compact layout: also shows the health badge next to the name, as an icon with an accessible name', () => {
  mockMatchMedia(true);
  mockScore(scoreFixture('moss'));
  renderHeader();

  // Issue #383: the compact layout renders an icon-only badge (not a text
  // Chip -- see ContactHeader.tsx's comment for why: a text Chip forces a
  // header wrap that was measured to break an unrelated a11y test on
  // narrow viewports), so "Healthy" is only reachable via the button's
  // accessible name here, never as visible text.
  expect(screen.getByRole('button', { name: 'Healthy' })).toBeInTheDocument();
  expect(screen.queryByText('Healthy')).not.toBeInTheDocument();
});

test('wide layout: clicking the health badge opens a popover with all five facet reasons', async () => {
  mockMatchMedia(false);
  mockScore(scoreFixture('moss'));
  renderHeader();

  expect(screen.queryByText('Relationship health breakdown')).not.toBeInTheDocument();

  fireEvent.click(screen.getByText('Healthy'));

  expect(await screen.findByText('Relationship health breakdown')).toBeInTheDocument();
  // Rendered from reason_code/reason_params in the UI language, never the
  // server's English "day(s)" sentence.
  expect(
    screen.getByText('Last qualifying interaction 5 days ago (target: every 14 days)'),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      '1 qualifying interaction in the last 180 days (about 12.9 expected at this cadence)',
    ),
  ).toBeInTheDocument();
  expect(screen.getByText('Directly connected as Spouse')).toBeInTheDocument();
  expect(screen.getByText('Contact record last edited 1 day ago')).toBeInTheDocument();
  expect(screen.queryByText(/day\(s\)/)).toBeNull();
  expect(screen.getByText('No pending reach-out suggestion')).toBeInTheDocument();
});

test('compact layout: clicking the health badge opens the same popover with all five facet reasons', async () => {
  mockMatchMedia(true);
  mockScore(scoreFixture('russula'));
  renderHeader();

  // Issue #383: compact layout's badge is icon-only -- click via its
  // accessible name, not visible text (see the test above).
  fireEvent.click(screen.getByRole('button', { name: 'Neglected' }));

  expect(await screen.findByText('Relationship health breakdown')).toBeInTheDocument();
  // Rendered from reason_code/reason_params in the UI language, never the
  // server's English "day(s)" sentence.
  expect(
    screen.getByText('Last qualifying interaction 5 days ago (target: every 14 days)'),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      '1 qualifying interaction in the last 180 days (about 12.9 expected at this cadence)',
    ),
  ).toBeInTheDocument();
  expect(screen.getByText('Directly connected as Spouse')).toBeInTheDocument();
  expect(screen.getByText('Contact record last edited 1 day ago')).toBeInTheDocument();
  expect(screen.queryByText(/day\(s\)/)).toBeNull();
  expect(screen.getByText('No pending reach-out suggestion')).toBeInTheDocument();
});

test('the health score popover closes on an outside click', async () => {
  mockMatchMedia(false);
  mockScore(scoreFixture('moss'));
  const { container } = renderHeader();

  fireEvent.click(screen.getByText('Healthy'));
  expect(await screen.findByText('Relationship health breakdown')).toBeInTheDocument();

  // MUI's Popover mounts an (invisible) Backdrop in the Modal it renders
  // into; clicking it is how a real user's outside click dismisses it.
  const backdrop = container.ownerDocument.querySelector('.MuiBackdrop-root');
  expect(backdrop).not.toBeNull();
  fireEvent.click(backdrop as Element);

  await waitFor(() =>
    expect(screen.queryByText('Relationship health breakdown')).not.toBeInTheDocument(),
  );
});

// --- Issue #1276: "Subscribe via feed" ------------------------------------

test('the overflow menu offers "Subscribe via feed" and reports the click (#1276)', () => {
  mockMatchMedia(true);
  const onSubscribeFeed = vi.fn();
  renderHeader({ onSubscribeFeed });

  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByText('Subscribe via feed'));

  expect(onSubscribeFeed).toHaveBeenCalledTimes(1);
});

test('the wide layout shows a "Subscribe via feed" item in the More menu (#1276)', () => {
  mockMatchMedia(false);
  const onSubscribeFeed = vi.fn();
  renderHeader({ onSubscribeFeed });

  fireEvent.click(screen.getByRole('button', { name: 'More' }));
  fireEvent.click(screen.getByRole('menuitem', { name: 'Subscribe via feed' }));

  expect(onSubscribeFeed).toHaveBeenCalledTimes(1);
});

test('no "Subscribe via feed" action without a handler (#1276)', () => {
  mockMatchMedia(true);
  renderHeader();
  fireEvent.click(screen.getByLabelText('Actions'));
  expect(screen.queryByText('Subscribe via feed')).not.toBeInTheDocument();
});

// --- "Add" button paths (the Enter-key path is covered above) --------------

test('clicking the circles Add button calls onAddCircle with a fresh circle', () => {
  mockMatchMedia(false);
  const onAddCircle = vi.fn();
  renderHeader({ onAddCircle });

  fireEvent.click(screen.getAllByLabelText('Edit')[1]);
  const input = screen.getByPlaceholderText('New circle name...');
  fireEvent.change(input, { target: { value: 'Book Club' } });
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  expect(onAddCircle).toHaveBeenCalledWith({
    id: '',
    created_at: '',
    updated_at: '',
    name: 'Book Club',
  });
  expect(input).toHaveValue('');
});

test('clicking the tags Add button calls onAddTag with a fresh tag', () => {
  mockMatchMedia(false);
  const onAddTag = vi.fn();
  renderHeader({ onAddTag });

  fireEvent.click(screen.getAllByLabelText('Edit')[2]);
  const input = screen.getByPlaceholderText('New tag...');
  fireEvent.change(input, { target: { value: 'Neighbor' } });
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  expect(onAddTag).toHaveBeenCalledWith({
    id: '',
    created_at: '',
    updated_at: '',
    name: 'Neighbor',
  });
  expect(input).toHaveValue('');
});

// --- export menu branches (wide layout) ------------------------------------

test('wide layout: choosing vCard 4.0 in the export menu reports the format', async () => {
  mockMatchMedia(false);
  const onExportContact = vi.fn();
  renderHeader({ onExportContact });

  fireEvent.click(screen.getByRole('button', { name: 'More' }));
  fireEvent.click(await screen.findByText('Export as vCard 4.0'));

  expect(onExportContact).toHaveBeenCalledWith('vcf4');
});

test('wide layout: choosing JSContact in the export menu reports the format', async () => {
  mockMatchMedia(false);
  const onExportContact = vi.fn();
  renderHeader({ onExportContact });

  fireEvent.click(screen.getByRole('button', { name: 'More' }));
  fireEvent.click(await screen.findByText('Export as JSContact'));

  expect(onExportContact).toHaveBeenCalledWith('jscontact');
});

// --- compact overflow menu action handlers ---------------------------------

test('compact overflow menu reports the Merge and Prep-view clicks', () => {
  mockMatchMedia(true);
  const onMergeContact = vi.fn();
  const onPrepView = vi.fn();
  renderHeader({ onMergeContact, onPrepView });

  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByText('Merge'));
  expect(onMergeContact).toHaveBeenCalledTimes(1);

  // Selecting a menu item closes the menu; reopen it for the next action.
  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByText('Prep view'));
  expect(onPrepView).toHaveBeenCalledTimes(1);
});

test('compact overflow menu reports the Share and export clicks', () => {
  mockMatchMedia(true);
  const onShareContact = vi.fn();
  const onExportContact = vi.fn();
  renderHeader({ onShareContact, onExportContact });

  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByText('Share Contact'));
  expect(onShareContact).toHaveBeenCalledTimes(1);

  fireEvent.click(screen.getByLabelText('Actions'));
  fireEvent.click(screen.getByText('Export as JSContact'));
  expect(onExportContact).toHaveBeenCalledWith('jscontact');
});

// Issue #1735: the score breakdown explains an ambient contact.
test.each([
  ['household', 'Ambient contact (lives with you)'],
  ['flag', 'Ambient contact (marked)'],
] as const)('score popover explains an ambient contact (%s)', async (reason, text) => {
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), ambient: true, ambient_reason: reason });
  renderHeader();

  fireEvent.click(screen.getByText('Healthy'));

  expect(await screen.findByText(text)).toBeInTheDocument();
});

test('score popover has no ambient note for a non-ambient contact', async () => {
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), ambient: false });
  renderHeader();

  fireEvent.click(screen.getByText('Healthy'));

  await screen.findByText('Relationship health breakdown');
  expect(screen.queryByTestId('ambient-explanation')).not.toBeInTheDocument();
});

// --- Issue #1797: target closeness chip, picker, cadence offer -----------

test('views a score with a target as a "Target: Close · Drifting" chip', () => {
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: targetFixture() });
  renderHeader();

  expect(screen.getByText('Target: Close · Drifting')).toBeInTheDocument();
});

test('renders no target chip when score.target is null', () => {
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: null });
  renderHeader();

  expect(screen.queryByText(/^Target:/)).not.toBeInTheDocument();
});

test('compact layout also shows the target chip', () => {
  mockMatchMedia(true);
  mockScore({
    ...scoreFixture('moss'),
    target: targetFixture({ tier: 'distant', status: 'behind' }),
  });
  renderHeader();

  expect(screen.getByText('Target: Distant · Behind')).toBeInTheDocument();
});

test('the target chip opens the same score popover', async () => {
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: targetFixture() });
  renderHeader({ onSetTargetCloseness: vi.fn() });

  fireEvent.click(screen.getByText('Target: Close · Drifting'));

  expect(await screen.findByText('Relationship health breakdown')).toBeInTheDocument();
  expect(
    screen.getByText('Recency and frequency score 30 against the Close target (every 14 days)'),
  ).toBeInTheDocument();
});

test('renders no target picker when onSetTargetCloseness is not provided', async () => {
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: targetFixture() });
  renderHeader();

  fireEvent.click(screen.getByText('Healthy'));

  await screen.findByText('Relationship health breakdown');
  expect(screen.queryByLabelText('Target closeness')).not.toBeInTheDocument();
});

test('choosing "Casual" calls onSetTargetCloseness with casual and refreshes the score', async () => {
  const onSetTargetCloseness = vi.fn().mockResolvedValue({
    contact_id: 1,
    target_closeness: 'casual',
    default_interval_days: 30,
    cadence_policy_offer: null,
  });
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: null });
  renderHeader({ onSetTargetCloseness });
  fireEvent.click(screen.getByText('Healthy'));
  await screen.findByText('Relationship health breakdown');

  fireEvent.mouseDown(screen.getByLabelText('Target closeness'));
  fireEvent.click(await screen.findByText('Casual'));

  await waitFor(() => expect(onSetTargetCloseness).toHaveBeenCalledWith('casual'));
  await waitFor(() => expect(refreshScoreMock).toHaveBeenCalledTimes(1));
  expect(screen.queryByText('Create a cadence?')).not.toBeInTheDocument();
});

test('choosing "No target" calls onSetTargetCloseness with null', async () => {
  const onSetTargetCloseness = vi.fn().mockResolvedValue({
    contact_id: 1,
    target_closeness: null,
    default_interval_days: null,
    cadence_policy_offer: null,
  });
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: null });
  renderHeader({
    record: { ...baseRecord(), crm: { kind: 'human', target_closeness: 'close' } },
    onSetTargetCloseness,
  });
  fireEvent.click(screen.getByText('Healthy'));
  await screen.findByText('Relationship health breakdown');

  fireEvent.mouseDown(screen.getByLabelText('Target closeness'));
  fireEvent.click(await screen.findByText('No target'));

  await waitFor(() => expect(onSetTargetCloseness).toHaveBeenCalledWith(null));
});

test('an offer in the response opens the cadence dialog with the name and interval', async () => {
  const onSetTargetCloseness = vi.fn().mockResolvedValue({
    contact_id: 1,
    target_closeness: 'close',
    default_interval_days: 14,
    cadence_policy_offer: offer,
  });
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: null });
  renderHeader({ onSetTargetCloseness });
  fireEvent.click(screen.getByText('Healthy'));
  await screen.findByText('Relationship health breakdown');

  fireEvent.mouseDown(screen.getByLabelText('Target closeness'));
  fireEvent.click(await screen.findByText('Close'));

  expect(await screen.findByText('Create a cadence?')).toBeInTheDocument();
  expect(screen.getByText('Stay in touch with Fluffy every 14 days?')).toBeInTheDocument();
});

test('"Create cadence" calls onAcceptCadenceOffer with the offer and closes', async () => {
  const onSetTargetCloseness = vi.fn().mockResolvedValue({
    contact_id: 1,
    target_closeness: 'close',
    default_interval_days: 14,
    cadence_policy_offer: offer,
  });
  const onAcceptCadenceOffer = vi.fn().mockResolvedValue(undefined);
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: null });
  renderHeader({ onSetTargetCloseness, onAcceptCadenceOffer });
  fireEvent.click(screen.getByText('Healthy'));
  await screen.findByText('Relationship health breakdown');

  fireEvent.mouseDown(screen.getByLabelText('Target closeness'));
  fireEvent.click(await screen.findByText('Close'));
  fireEvent.click(await screen.findByRole('button', { name: 'Create cadence' }));

  await waitFor(() => expect(onAcceptCadenceOffer).toHaveBeenCalledWith(offer));
  await waitFor(() => expect(screen.queryByText('Create a cadence?')).not.toBeInTheDocument());
  // Once after the picker write, once after the accepted cadence.
  expect(refreshScoreMock).toHaveBeenCalledTimes(2);
});

test('"Not now" closes the cadence offer without calling onAcceptCadenceOffer', async () => {
  const onSetTargetCloseness = vi.fn().mockResolvedValue({
    contact_id: 1,
    target_closeness: 'close',
    default_interval_days: 14,
    cadence_policy_offer: offer,
  });
  const onAcceptCadenceOffer = vi.fn().mockResolvedValue(undefined);
  mockMatchMedia(false);
  mockScore({ ...scoreFixture('moss'), target: null });
  renderHeader({ onSetTargetCloseness, onAcceptCadenceOffer });
  fireEvent.click(screen.getByText('Healthy'));
  await screen.findByText('Relationship health breakdown');

  fireEvent.mouseDown(screen.getByLabelText('Target closeness'));
  fireEvent.click(await screen.findByText('Close'));
  fireEvent.click(await screen.findByRole('button', { name: 'Not now' }));

  await waitFor(() => expect(screen.queryByText('Create a cadence?')).not.toBeInTheDocument());
  expect(onAcceptCadenceOffer).not.toHaveBeenCalled();
  // Only the picker write refreshed the score; dismissing makes no request.
  expect(refreshScoreMock).toHaveBeenCalledTimes(1);
});

test('the score is refetched when crm.ambient changes', () => {
  mockMatchMedia(false);
  mockScore(scoreFixture('moss'));
  renderHeader({ record: { ...baseRecord(), crm: { kind: 'human', ambient: true } } });

  expect(useContactScore).toHaveBeenCalledWith(expect.anything(), true);
});
