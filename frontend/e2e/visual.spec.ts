// e2e/visual.spec.ts
//
// Screenshot-based visual regression for a small, curated set of stable views
// (issue #258): the dashboard, the contacts list, a contact detail page and
// the "Add reminder" dialog, at desktop and phone widths. An unintended
// layout/theme change to any pinned view fails CI as a pixel diff instead of
// shipping silently.
//
// Baselines live next to this file in `visual.spec.ts-snapshots/` and are
// committed to the repository. Regenerate them after an *intentional* visual
// change with:
//
//     npx playwright test visual.spec.ts --update-snapshots
//
// then eyeball the regenerated images (the HTML report / `--ui` shows the
// diff) before committing the new baselines. Snapshot files are suffixed
// `-chromium-linux` — that is CI's platform, and the only suffix that should
// be committed. Regenerating on another OS produces a differently-suffixed
// file; don't delete the committed `-linux` baselines to replace them with
// those.
//
// Keep the surface small (5-10 shots, as the ticket says): every pinned view
// is a baseline the next intentional redesign has to regenerate, so adding a
// shot here is a maintenance commitment.
//
// Why each piece of scaffolding below exists — a visual baseline is only
// worth anything if it renders byte-identically on the machine that generated
// it and in CI, otherwise the gate is noise:
//
//  * Fonts: the app's type stack is a *system* font stack (IBM Plex Sans →
//    Fira Sans → … → sans-serif, see src/theme.ts), so text rendering depends
//    on which fonts the host has installed. A baseline generated on a machine
//    with IBM Plex Sans installed would differ from CI's DejaVu Sans fallback
//    on every single glyph. Rather than chase host font packages, every test
//    registers the app's own primary family (IBM Plex Sans, latin subset,
//    OFL-licensed) as a committed @font-face served by a route interception
//    (fixtures/fonts/), so the identical woff2 renders on every host. The
//    font must actually load before a snapshot — asserted, and waited on, so
//    a broken route fails with a clear message instead of a confusing diff.
//  * Clock: every date the UI renders (birthday ages, reminder dates, the
//    dialog's default date) is pinned with page.clock.setFixedTime so a
//    baseline can never expire on a different day or year.
//  * Data: the dashboard and the contacts list are intercepted with fixed
//    wire responses. The dashboard's "Stay in Touch" column is picked
//    randomly server-side on every load, and the list's rows can be
//    transiently altered by any other spec running under fullyParallel —
//    shared test-user state must never be allowed to move a baseline. The
//    contact-detail and dialog shots use a spec-owned throwaway contact
//    instead (created and deleted by the test), so they stay on the real
//    API path with data nothing else can touch.
//
// The automatic per-test a11y scan (fixtures.ts) still runs after each shot,
// so these views stay double-gated: pixel-identical *and* axe-clean.

import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import type { Page } from '@playwright/test';
import type { Circle, CircleMember } from '../src/api/circles';
import type { Birthday, Contact, ContactSummaryDTO } from '../src/api/contacts';
import type { DashboardReminder } from '../src/api/dashboard';
import {
  createTestContact,
  deleteTestContact,
  expect,
  stableClick,
  test,
  waitForLoading,
} from './fixtures';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// ---------------------------------------------------------------------------
// Pinned rendering environment
// ---------------------------------------------------------------------------

// All UI-visible dates below are chosen relative to this instant so age text,
// reminder dates and the dialog's default date render identically forever.
const FROZEN_TIME = new Date('2026-08-15T12:00:00Z');

// The app's aria-live announcer (App.tsx #211) is visually hidden with
// `clip: inset(50%)`, but its layout box (a full viewport tall) still counts
// toward document height, so a fullPage screenshot would be padded by a big
// blank region. Normalize it to a 1px fixed box for the capture — it stays in
// the a11y tree, it just stops inflating the shot. Must come after the
// @font-face (same injected <style>).
const PINNED_CSS = `
@font-face {
  font-family: 'IBM Plex Sans';
  font-style: normal;
  font-weight: 100 700;
  font-display: block;
  src: url('/__visual_fonts__/ibm-plex-sans-latin.woff2') format('woff2');
}
@font-face {
  font-family: 'IBM Plex Sans';
  font-style: italic;
  font-weight: 100 700;
  font-display: block;
  src: url('/__visual_fonts__/ibm-plex-sans-latin-italic.woff2') format('woff2');
}
[aria-live="polite"] {
  position: fixed;
  top: 0;
  left: 0;
  width: 1px;
  height: 1px;
  overflow: hidden;
}
`;

/**
 * Routes the pinned webfont files and freezes the page clock. Must run before
 * page.goto(); call `awaitPinFonts(page)` after the page has loaded so the
 * @font-face is registered (and the font fetched) before any snapshot.
 */
async function pinVisualEnvironment(page: Page): Promise<void> {
  await page.route('**/__visual_fonts__/ibm-plex-sans-latin.woff2', (route) =>
    route.fulfill({
      path: path.join(__dirname, 'fixtures', 'fonts', 'ibm-plex-sans-latin.woff2'),
      contentType: 'font/woff2',
    }),
  );
  await page.route('**/__visual_fonts__/ibm-plex-sans-latin-italic.woff2', (route) =>
    route.fulfill({
      path: path.join(__dirname, 'fixtures', 'fonts', 'ibm-plex-sans-latin-italic.woff2'),
      contentType: 'font/woff2',
    }),
  );
  await page.clock.setFixedTime(FROZEN_TIME);
}

/** Registers the pinned @font-face and waits until it has actually loaded. */
async function awaitPinFonts(page: Page): Promise<void> {
  await page.addStyleTag({ content: PINNED_CSS });
  await page.evaluate(() => document.fonts.ready);
  const loaded = await page.evaluate(() => document.fonts.check('16px "IBM Plex Sans"'));
  expect(
    loaded,
    'the pinned IBM Plex Sans webfont must load before a visual snapshot — check ' +
      'that the __visual_fonts__ route is registered before page.goto()',
  ).toBe(true);
}

// ---------------------------------------------------------------------------
// Fixed wire responses — the views' data, decoupled from shared test-user
// state and from the random/date content the real backend would return.
// ---------------------------------------------------------------------------

const FIXED_CIRCLES: Circle[] = [
  {
    id: 'circle-friends',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    name: 'Friends',
  },
  {
    id: 'circle-work',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    name: 'Work',
  },
  {
    id: 'circle-family',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    name: 'Family',
  },
];

const FIXED_MEMBERS: CircleMember[] = [
  { id: 1, circle_id: 'circle-friends', member_vcard_uid: 'uid-alice-johnson' },
  { id: 2, circle_id: 'circle-work', member_vcard_uid: 'uid-alice-johnson' },
  { id: 3, circle_id: 'circle-family', member_vcard_uid: 'uid-bob-smith' },
];

// Slim GET /contacts list projection (ContactSummaryDTO) — five rows covering
// email/phone/birthday/org presence and a favorite, so the list renders every
// row affordance (star, chips, initials) without relying on the seeded data.
const FIXED_CONTACT_SUMMARIES: ContactSummaryDTO[] = [
  {
    id: 1,
    uid: 'uid-alice-johnson',
    firstname: 'Alice',
    lastname: 'Johnson',
    nickname: '',
    fn: 'Alice Johnson',
    primary_email: 'alice@example.com',
    primary_phone: '+1 555-0101',
    birthday: '1990-03-15',
    org: 'Acme Corp',
    photo: '',
    photo_thumbnail: '',
    archived: false,
    is_favorite: true,
  },
  {
    id: 2,
    uid: 'uid-bob-smith',
    firstname: 'Bob',
    lastname: 'Smith',
    nickname: '',
    fn: 'Bob Smith',
    primary_email: 'bob@example.com',
    primary_phone: '',
    birthday: '',
    org: '',
    photo: '',
    photo_thumbnail: '',
    archived: false,
    is_favorite: false,
  },
  {
    id: 3,
    uid: 'uid-carol-williams',
    firstname: 'Carol',
    lastname: 'Williams',
    nickname: '',
    fn: 'Carol Williams',
    primary_email: 'carol@example.com',
    primary_phone: '',
    birthday: '1985-07-22',
    org: '',
    photo: '',
    photo_thumbnail: '',
    archived: false,
    is_favorite: false,
  },
  {
    id: 4,
    uid: 'uid-david-brown',
    firstname: 'David',
    lastname: 'Brown',
    nickname: '',
    fn: 'David Brown',
    primary_email: 'david@example.com',
    primary_phone: '',
    birthday: '',
    org: 'Globex',
    photo: '',
    photo_thumbnail: '',
    archived: false,
    is_favorite: false,
  },
  {
    id: 5,
    uid: 'uid-eve-davis',
    firstname: 'Eve',
    lastname: 'Davis',
    nickname: '',
    fn: 'Eve Davis',
    primary_email: '',
    primary_phone: '+1 555-0105',
    birthday: '',
    org: '',
    photo: '',
    photo_thumbnail: '',
    archived: false,
    is_favorite: false,
  },
];

// The dashboard composite (GET /dashboard). overdue/reach_out_suggestions/
// contact_sync_conflicts stay empty so those blocks render nothing at all —
// the "all-clear dashboard stays clean" rule — while favorites/birthdays/
// reminders/random fill their columns with a representative mix.
function legacyContact(
  overrides: Partial<Contact> & Pick<Contact, 'ID' | 'firstname' | 'lastname'>,
): Contact {
  return {
    nickname: '',
    email: '',
    phone: '',
    photo_thumbnail: '',
    archived: false,
    is_favorite: false,
    ...overrides,
  };
}

const FIXED_FAVORITES: Contact[] = [
  legacyContact({
    ID: 1,
    uid: 'uid-alice-johnson',
    firstname: 'Alice',
    lastname: 'Johnson',
    email: 'alice@example.com',
    is_favorite: true,
  }),
  legacyContact({ ID: 2, uid: 'uid-bob-smith', firstname: 'Bob', lastname: 'Smith' }),
];

const FIXED_BIRTHDAYS: Birthday[] = [
  { type: 'contact', name: 'Alice Johnson', birthday: '1990-03-15', contact_id: 1 },
  { type: 'contact', name: 'Bob Smith', birthday: '1988-11-02', contact_id: 2 },
];

const FIXED_REMINDERS: DashboardReminder[] = [
  {
    ID: 11,
    message: 'Call Bob about the hiking trip',
    by_mail: true,
    remind_at: '2026-08-20T09:00:00Z',
    recurrence: 'once',
    reoccur_from_completion: true,
    completed: false,
    email_sent: false,
    contact_id: 2,
    contact_name: 'Bob Smith',
  },
  {
    ID: 12,
    message: 'Water the plants at home',
    by_mail: false,
    remind_at: '2026-08-29T18:00:00Z',
    recurrence: 'weekly',
    reoccur_from_completion: true,
    completed: false,
    email_sent: false,
    contact_id: 3,
    contact_name: 'Carol Williams',
  },
];

const FIXED_RANDOM_CONTACTS: Contact[] = [
  legacyContact({ ID: 5, uid: 'uid-eve-davis', firstname: 'Eve', lastname: 'Davis' }),
  legacyContact({ ID: 4, uid: 'uid-david-brown', firstname: 'David', lastname: 'Brown' }),
  legacyContact({ ID: 3, uid: 'uid-carol-williams', firstname: 'Carol', lastname: 'Williams' }),
];

const FIXED_DASHBOARD = {
  birthdays: FIXED_BIRTHDAYS,
  random_contacts: FIXED_RANDOM_CONTACTS,
  upcoming_reminders: FIXED_REMINDERS,
  overdue: [],
  favorites: FIXED_FAVORITES,
  reach_out_suggestions: [],
  contact_sync_conflicts: [],
};

/** Fixed circles response — drives the list's filter dropdown and row chips. */
async function interceptCircles(page: Page): Promise<void> {
  await page.route('**/api/v1/circles?*', (route) =>
    route.fulfill({
      json: {
        circles: FIXED_CIRCLES,
        total: FIXED_CIRCLES.length,
        next_cursor: '',
        limit: 200,
        members: FIXED_MEMBERS,
      },
    }),
  );
}

/** Fixed GET /contacts list response, decoupled from parallel-spec DB churn. */
async function interceptContactsList(page: Page): Promise<void> {
  await page.route('**/api/v1/contacts?*', (route) =>
    route.fulfill({
      json: { contacts: FIXED_CONTACT_SUMMARIES, next_cursor: '', limit: 10 },
    }),
  );
}

// ---------------------------------------------------------------------------
// Desktop (1280x720 — the chromium project's pinned device/viewport)
// ---------------------------------------------------------------------------

test.describe('Visual regression (desktop)', () => {
  test.use({ viewport: { width: 1280, height: 720 } });

  test('dashboard', async ({ page }) => {
    await pinVisualEnvironment(page);
    await interceptCircles(page);
    await page.route('**/api/v1/dashboard', (route) => route.fulfill({ json: FIXED_DASHBOARD }));

    await page.goto('/');
    await awaitPinFonts(page);
    await waitForLoading(page);

    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
    await expect(page).toHaveScreenshot('desktop-dashboard.png', { fullPage: true });
  });

  test('contacts list', async ({ page }) => {
    await pinVisualEnvironment(page);
    await interceptCircles(page);
    await interceptContactsList(page);

    await page.goto('/contacts');
    await awaitPinFonts(page);
    await waitForLoading(page);

    await expect(page.getByRole('heading', { name: /contacts/i })).toBeVisible();
    await expect(page.getByText('Alice Johnson')).toBeVisible();
    await expect(page).toHaveScreenshot('desktop-contacts-list.png', { fullPage: true });
  });

  test('contact detail', async ({ page }) => {
    await pinVisualEnvironment(page);
    const contact = await createTestContact(page.request, {
      firstname: 'E2EFixtureVera',
      lastname: 'Visual',
      emails: [{ type: 'home', value: 'vera.visual@example.com' }],
      phones: [{ type: 'mobile', value: '+1 555-0199' }],
      birthday: '1988-05-20',
      circles: ['Friends'],
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await awaitPinFonts(page);
      await waitForLoading(page);

      await expect(page.getByRole('heading', { name: /Vera Visual/i })).toBeVisible();
      await expect(page).toHaveScreenshot('desktop-contact-detail.png');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('add reminder dialog', async ({ page }) => {
    await pinVisualEnvironment(page);
    const contact = await createTestContact(page.request, {
      firstname: 'E2EFixtureVera',
      lastname: 'Visual',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await awaitPinFonts(page);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: /add.*reminder/i }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await expect(dialog).toHaveScreenshot('desktop-add-reminder-dialog.png');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});

// ---------------------------------------------------------------------------
// Phone width (390x844) — the responsive layout the a11y + T31/T45 suites
// guard functionally; these pin it visually.
// ---------------------------------------------------------------------------

test.describe('Visual regression (mobile)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('dashboard', async ({ page }) => {
    await pinVisualEnvironment(page);
    await interceptCircles(page);
    await page.route('**/api/v1/dashboard', (route) => route.fulfill({ json: FIXED_DASHBOARD }));

    await page.goto('/');
    await awaitPinFonts(page);
    await waitForLoading(page);

    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
    await expect(page).toHaveScreenshot('mobile-dashboard.png', { fullPage: true });
  });

  test('contact detail', async ({ page }) => {
    await pinVisualEnvironment(page);
    const contact = await createTestContact(page.request, {
      firstname: 'E2EFixtureVera',
      lastname: 'Visual',
      emails: [{ type: 'home', value: 'vera.visual@example.com' }],
      phones: [{ type: 'mobile', value: '+1 555-0199' }],
      birthday: '1988-05-20',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await awaitPinFonts(page);
      await waitForLoading(page);

      await expect(page.getByRole('heading', { name: /Vera Visual/i })).toBeVisible();
      await expect(page).toHaveScreenshot('mobile-contact-detail.png');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
