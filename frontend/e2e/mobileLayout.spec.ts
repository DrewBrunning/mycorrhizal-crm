import type { Page } from '@playwright/test';
import { createTestContact, deleteTestContact, expect, test, uniqueToken } from './fixtures';
import { API_BASE_URL } from './global-setup';

// T32: Network, Settings and User Management were found broken at phone widths
// during real-world testing — the same class of fix T28 applied to the contact
// page. These specs pin the invariant the ticket demands: none of the three
// pages overflows the page horizontally at phone widths, and User Management's
// user list stays readable and actionable (stacked cards, not a clipped table).
async function noHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflow, 'the page must not scroll horizontally').toBe(false);
}

test.describe('Mobile layout: Network (T32)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('graph is viewable without page-level horizontal overflow', async ({ page }) => {
    await page.goto('/network');
    await expect(page.getByRole('heading', { name: /contact network/i })).toBeVisible();
    // Seeded contacts exist, so the graph canvas renders.
    await expect(page.locator('canvas').first()).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

test.describe('Mobile layout: Settings (T32)', () => {
  test.use({ viewport: { width: 360, height: 800 } });

  test('sections reflow without page-level horizontal overflow', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.getByRole('heading', { name: /settings/i })).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

test.describe('Mobile layout: User Management (T32)', () => {
  // The shared `testuser` is the auto-admin; per-worker users are not.
  test.use({ viewport: { width: 360, height: 800 }, sharedUser: true });

  test('user list reflows to stacked cards, readable and actionable', async ({ page }) => {
    await page.goto('/users');
    await expect(page.getByRole('heading', { name: /user management/i })).toBeVisible();

    // The seeded test user is the first registered account (auto-admin), so
    // it appears in the list. At phone width it renders as a card, not a table.
    await expect(page.getByText('testuser', { exact: true })).toBeVisible();
    await expect(page.getByRole('table')).toHaveCount(0);
    // Edit/delete actions stay reachable on the stacked layout.
    await expect(page.getByTitle('Edit').first()).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

test.describe('Mobile layout: User Management at 414px (T32)', () => {
  test.use({ viewport: { width: 414, height: 896 }, sharedUser: true });

  test('user list does not overflow at 414px either', async ({ page }) => {
    await page.goto('/users');
    await expect(page.getByRole('heading', { name: /user management/i })).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

// Issue #1785: page-header action buttons used to be uppercase MUI Buttons in a
// non-wrapping row, so long German/French labels pushed the page wider than the
// viewport ("AKTUALISIEREN" +50px on /system-status, "DIAGNOSE AUSFÜHREN" +12px
// on /system-events, the triage stepper +8px, /activities +5px). The shared
// PageHeader wraps and shrinks its actions; this sweep pins the outcome in a real
// browser for every header-bearing route in every shipped locale.
const HEADER_ROUTES = [
  '/contacts',
  '/activities',
  '/notes',
  '/occasions',
  '/households',
  '/circles',
  '/shares',
  '/audit',
  '/settings',
  '/settings/data',
  '/users',
  '/system-events',
  '/system-status',
  '/circle-tag-triage',
];
const HEADER_LOCALES = ['de', 'fr', 'es', 'it', 'en'];
const HEADER_WIDTHS = [320, 360];

async function documentOverflowPx(page: Page): Promise<number> {
  return page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
}

test.describe('Mobile layout: page headers wrap in every locale (#1785)', () => {
  // The shared `testuser` is the auto-admin, so the admin-only routes render.
  test.use({ sharedUser: true });

  for (const locale of HEADER_LOCALES) {
    for (const width of HEADER_WIDTHS) {
      for (const route of HEADER_ROUTES) {
        test(`${route} has no horizontal overflow at ${width}px in ${locale}`, async ({ page }) => {
          // i18next's language detector reads localStorage before navigator.
          await page.addInitScript((lng) => {
            localStorage.setItem('i18nextLng', lng);
          }, locale);
          await page.setViewportSize({ width, height: 800 });
          await page.goto(route);
          await expect(page.locator('h1').first(), `${route} renders its heading`).toBeVisible();
          // The sweep is only a locale sweep if the locale actually applied.
          await expect(page.locator('html')).toHaveAttribute('lang', locale);
          // Let the lazily loaded panels (system events, triage) lay out.
          await page.waitForLoadState('networkidle');
          expect(await documentOverflowPx(page), 'page overflows horizontally (px)').toBe(0);
          // Header actions stay reachable: phone touch targets are >= 44px.
          for (const button of await page
            .getByTestId('page-header-actions')
            .locator('button')
            .all()) {
            const box = await button.boundingBox();
            if (box) expect(box.height, 'header action height').toBeGreaterThanOrEqual(44);
          }
          // ...and so are icon buttons beside the title (the Activities/Notes info button).
          for (const iconButton of await page
            .getByTestId('page-header')
            .locator('.MuiIconButton-root')
            .all()) {
            const box = await iconButton.boundingBox();
            if (box) {
              expect(box.height, 'header icon button height').toBeGreaterThanOrEqual(44);
              expect(box.width, 'header icon button width').toBeGreaterThanOrEqual(44);
            }
          }
        });
      }
    }
  }
});

// Issues #1780/#1781: Add Contact is a long form with two three-control rows.
// On a phone it goes full-screen, focuses the first field, and the
// existing-circle/tag pickers each get their own full-width line -- they used
// to be squeezed next to the "Or create new..." field and Add button until
// only the dropdown arrow was visible.
test.describe('Mobile layout: Add Contact (#1780, #1781)', () => {
  for (const width of [320, 360, 390]) {
    test(`full-screen with usable Circle/Tag pickers at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 });
      await page.goto('/contacts');
      await page.getByRole('button', { name: /add/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      // Issue #1780: the caret starts in First Name.
      await expect(page.getByLabel(/first.*name/i)).toBeFocused();

      // Issue #1781: below `sm` the dialog paper fills the viewport instead of
      // remaining a centred modal with wide margins. MUI's role="dialog"
      // element is the paper itself, so measure the dialog locator directly.
      const paperBox = await dialog.boundingBox();
      expect(paperBox, 'dialog paper is laid out').not.toBeNull();
      // Allow for a classic scrollbar the modal may reserve.
      expect(paperBox?.width).toBeGreaterThanOrEqual(width - 20);

      // Both "select existing..." pickers are usable, not collapsed to an arrow.
      for (const label of [/select existing circle/i, /select existing tag/i]) {
        const pickerBox = await dialog.getByLabel(label).boundingBox();
        expect(pickerBox, `${label} is laid out`).not.toBeNull();
        expect(pickerBox?.width, `${label} is usable`).toBeGreaterThanOrEqual(200);
      }

      await noHorizontalOverflow(page);
    });
  }
});

// ---------------------------------------------------------------------------
// Issues #2677, #2704, #2684: long names and chip rows must reflow at phone
// widths without overlapping or scrolling the page sideways.
// ---------------------------------------------------------------------------

// A 160-character name with a long unbreakable run, plus a non-Latin tail.
const LONG_NAME = `Bartholomew-Featherstonehaugh-Worthington-Smythe ${'Ab'.repeat(40)} Иванов Петров`;

interface Rect {
  left: number;
  right: number;
  top: number;
  bottom: number;
}

function overlaps(a: Rect, b: Rect): boolean {
  return (
    a.left < b.right - 1 && a.right > b.left + 1 && a.top < b.bottom - 1 && a.bottom > b.top + 1
  );
}

for (const width of [320, 375] as const) {
  test.describe(`Long names and chip rows at ${width}px (#2677 #2704 #2684)`, () => {
    test.use({ viewport: { width, height: 800 } });

    test('custom field rows keep their actions clear of the chips (#2677)', async ({ page }) => {
      const label = `Field ${uniqueToken()}`;
      const create = await page.request.post(`${API_BASE_URL}/field-definitions`, {
        data: {
          label,
          key: `k_${uniqueToken().toLowerCase()}`,
          target: 'organisation',
          type: 'string',
        },
      });
      expect(create.ok(), `create field definition: ${create.status()}`).toBeTruthy();
      const id = (await create.json()).field_definition.id as string;
      try {
        await page.goto('/settings/data');
        const row = page.getByRole('listitem').filter({ hasText: label });
        await expect(row).toBeVisible();
        const chips = await row.locator('.MuiChip-root').evaluateAll((els) =>
          els.map((e) => {
            const r = e.getBoundingClientRect();
            return { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
          }),
        );
        const actions = await row.getByRole('button').evaluateAll((els) =>
          els.map((e) => {
            const r = e.getBoundingClientRect();
            return { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
          }),
        );
        expect(chips.length).toBeGreaterThan(1);
        for (const chip of chips) {
          for (const action of actions) {
            expect(overlaps(chip, action), 'a row action overlaps a chip').toBe(false);
          }
        }
        expect(await documentOverflowPx(page), 'page overflows horizontally (px)').toBe(0);
      } finally {
        await page.request.delete(`${API_BASE_URL}/field-definitions/${id}`).catch(() => {});
      }
    });

    test('a long contact name does not widen /notes (#2704) or run under the edit pencil (#2684)', async ({
      page,
    }) => {
      const contact = await createTestContact(page.request, {
        firstname: LONG_NAME,
        lastname: 'Smythe',
      });
      try {
        const note = await page.request.post(`${API_BASE_URL}/contacts/${contact.ID}/notes`, {
          data: {
            content: 'A note about a person with a very long name',
            date: '2026-11-01T12:00:00Z',
          },
        });
        expect(note.ok(), `create note: ${note.status()}`).toBeTruthy();

        await page.goto('/notes');
        await expect(page.getByText('A note about a person with a very long name')).toBeVisible();
        expect(await documentOverflowPx(page), '/notes overflows horizontally (px)').toBe(0);

        await page.goto(`/contacts/${contact.ID}`);
        const heading = page.getByRole('heading', { level: 1 });
        await expect(heading).toBeVisible();
        expect(await documentOverflowPx(page), 'contact page overflows horizontally (px)').toBe(0);
        const textBox = await heading.evaluate((h) => {
          const range = document.createRange();
          range.selectNodeContents(h);
          const rects = Array.from(range.getClientRects());
          return {
            left: Math.min(...rects.map((r) => r.left)),
            right: Math.max(...rects.map((r) => r.right)),
            top: Math.min(...rects.map((r) => r.top)),
            bottom: Math.max(...rects.map((r) => r.bottom)),
          };
        });
        const pencil = await page.locator('.edit-icon').first().boundingBox();
        if (pencil) {
          expect(
            overlaps(textBox, {
              left: pencil.x,
              right: pencil.x + pencil.width,
              top: pencil.y,
              bottom: pencil.y + pencil.height,
            }),
            'the edit pencil overlaps the contact name',
          ).toBe(false);
        }
      } finally {
        await deleteTestContact(page.request, contact.ID);
      }
    });
  });
}
