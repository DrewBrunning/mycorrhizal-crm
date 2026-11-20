import { createTheme, ThemeProvider } from '@mui/material/styles';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import '../i18n/config';
import type { ContactRecordResponse } from '../api/contacts';
import { DateFormatProvider } from '../DateFormatProvider';
import { useContactScore } from '../hooks/useContactScore';
import ContactHeader from './ContactHeader';

// Real-browser tier (issue #2539): these assertions need an actual layout
// engine. The jsdom suite (ContactHeader.test.tsx) has to mock
// window.matchMedia, so it only proves "whatever the mock says"; this file
// drives the real media query at two real viewport widths and lets MUI's
// useMediaQuery read the browser's own answer. Never mock matchMedia here.
//
// frontend/AGENTS.md trap 1: no auto-cleanup and no `globals: true` -- the
// explicit afterEach(cleanup) applies in browser mode too.
afterEach(cleanup);

// ContactHeader fetches its own health score. Data, not layout: mocked so a
// missing backend cannot make a layout assertion flaky.
vi.mock('../hooks/useContactScore', () => ({ useContactScore: vi.fn() }));

beforeEach(() => {
  vi.mocked(useContactScore).mockReturnValue({
    score: null,
    loading: false,
    error: null,
    refreshScore: vi.fn(async () => {}),
  });
});

const MOBILE = { width: 375, height: 812 };
const DESKTOP = { width: 1280, height: 800 };

// MUI's default `lg` breakpoint is 1200px, and ContactHeader collapses its
// actions at `theme.breakpoints.down('lg')` -> `(max-width: 1199.95px)`.
const COMPACT_QUERY = '(max-width:1199.95px)';

function baseRecord(): ContactRecordResponse {
  return {
    id: 1,
    uid: 'uid-1',
    etag: '',
    revision: 1,
    card: { name: { components: [{ kind: 'given', value: 'Alice' }] } },
    crm: { kind: 'human' },
  };
}

function renderHeader() {
  return render(
    <ThemeProvider theme={createTheme()}>
      <DateFormatProvider>
        <ContactHeader
          record={baseRecord()}
          profilePic=""
          editingProfile={false}
          profileValues={{
            prefix: '',
            firstname: 'Alice',
            middle_name: '',
            lastname: '',
            suffix: '',
            nickname: '',
            kind: 'human',
            cardKind: '',
            language: '',
          }}
          contactCircles={[]}
          contactTags={[]}
          allCircles={[]}
          allTags={[]}
          onStartEditProfile={vi.fn()}
          onCancelEditProfile={vi.fn()}
          onSaveProfile={vi.fn()}
          onDeleteContact={vi.fn()}
          onProfileValueChange={vi.fn()}
          onAddCircle={vi.fn()}
          onRemoveCircle={vi.fn()}
          onAddTag={vi.fn()}
          onRemoveTag={vi.fn()}
          onUploadProfilePicture={vi.fn()}
          onStayInTouch={vi.fn()}
          onMergeContact={vi.fn()}
          onArchiveContact={vi.fn()}
          onExportContact={vi.fn()}
        />
      </DateFormatProvider>
    </ThemeProvider>,
  );
}

test('collapses the header actions into the overflow menu at a 375px viewport (T28)', async () => {
  await page.viewport(MOBILE.width, MOBILE.height);
  renderHeader();

  // The browser's own media query answers -- no mock anywhere in this file.
  expect(window.matchMedia(COMPACT_QUERY).matches).toBe(true);

  expect(screen.getByLabelText('Actions')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Stay in Touch' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'More' })).not.toBeInTheDocument();
});

test('renders the inline action buttons at a 1280px viewport (T28)', async () => {
  await page.viewport(DESKTOP.width, DESKTOP.height);
  renderHeader();

  expect(window.matchMedia(COMPACT_QUERY).matches).toBe(false);

  // Issue #1782: the three primary actions are inline at wide widths; the
  // rest live behind "More". The compact icon-only trigger is gone.
  expect(screen.getByRole('button', { name: 'Stay in Touch' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'More' })).toBeInTheDocument();
  expect(screen.queryByLabelText('Actions')).not.toBeInTheDocument();
});

test('the two layouts put the actions in measurably different places', async () => {
  await page.viewport(MOBILE.width, MOBILE.height);
  const mobile = renderHeader();
  const mobileTrigger = screen.getByLabelText('Actions').getBoundingClientRect();
  // The compact trigger is pinned to the header card's right edge, on its
  // own row -- not inline with the name.
  expect(mobileTrigger.width).toBeGreaterThan(0);
  expect(mobileTrigger.height).toBeGreaterThan(0);
  mobile.unmount();

  await page.viewport(DESKTOP.width, DESKTOP.height);
  renderHeader();
  const desktopActions = screen
    .getByRole('button', { name: 'Stay in Touch' })
    .getBoundingClientRect();

  // A real layout gives the wide-layout action row real width; the compact
  // icon-only trigger is narrower than any inline text button.
  expect(desktopActions.width).toBeGreaterThan(mobileTrigger.width);
});
