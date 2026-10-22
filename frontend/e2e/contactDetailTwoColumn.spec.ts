import type { Locator, Page } from '@playwright/test';
import { DEFAULT_ENABLED_CONTACT_FIELDS } from '../src/contactFields';
import { createTestContact, deleteTestContact, expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

// T74: field action buttons (edit/call/copy) sit too far from their field on
// wide desktop screens. The fix is two levels, both gated to lg (1200px)+:
//   Level 1 — ContactInformation's ~30 field groups flow into a two-column
//             grid inside their single card, so a field's right-anchored
//             action cluster lands ~530px from its value instead of ~1136px.
//   Level 2 — the people/timeline/cadence sections lay their PanelCards out
//             2-up in a grid (single-card sections and everything below lg
//             render exactly as before).
//
// Two later changes altered guards in this file, so the history is worth
// keeping straight:
//   - T109 (2026-08-15) moved the edit pencil from the far right of the field
//     row to beside the field-name label, which obsoleted every
//     "action-cluster gap" proxy for the two-column grid. The guards now use
//     row *width* (single-column rows span the full card; half-column rows
//     don't) instead of edit-button distance.
//   - The 2026-08-15 testing round dropped `fullWidth` from the Connections
//     panel (it is a list, not the graph), so Relationships and Connections
//     are now equal half-columns in the "people" section.
//
// These specs measure real geometry (bounding boxes) rather than asserting
// classes, because the complaint the ticket fixes is a *distance* and only
// layout can prove it's actually shorter.
function fieldRow(page: Page, label: string): Locator {
  // ContactInformation rows are icon | content Box. T109 moved the edit
  // pencil into the caption's own flex wrapper, so the caption is now three
  // boxes up from the row root (caption -> label row -> content box -> row).
  return page.getByText(label, { exact: true }).first().locator('..').locator('..').locator('..');
}

test.describe('T74 field action distance — Level 1, field grid', () => {
  test('phone actions sit near the value in two columns at 1440px', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      phones: [{ type: 'mobile', value: '+1 555-0101' }],
      emails: [{ type: 'home', value: 'alice@example.com' }],
      birthday: '1990-03-15',
    });

    try {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const row = fieldRow(page, 'Phone');
      const value = row.locator('a[href^="tel:"]').first();
      const edit = row.getByRole('button', { name: 'Edit' });

      const [valueBox, editBox] = await Promise.all([value.boundingBox(), edit.boundingBox()]);
      expect(valueBox && editBox, 'phone value and edit button both on screen').toBeTruthy();
      const gap = editBox?.x - valueBox?.x;
      console.log(`T74: phone value→edit gap at 1440px = ${Math.round(gap)}px`);
      // Pre-fix this gap was ~1100px (a full-width ~1136px row); two columns
      // put it under ~570px. 700px is a comfortable halfway-ish bound that
      // fails loudly if the grid ever stops being applied.
      expect(
        gap,
        `phone action cluster should sit near its value (gap ${Math.round(gap)}px)`,
      ).toBeLessThan(700);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('two enabled fields share a grid row in separate columns at lg+', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      phones: [{ type: 'mobile', value: '+1 555-0101' }],
      emails: [{ type: 'home', value: 'alice@example.com' }],
    });

    try {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // Phone and Address are the first two Contact-group rows in the grid,
      // so at lg+ they sit side by side in the same row.
      const phone = fieldRow(page, 'Phone');
      const address = fieldRow(page, 'Address');
      const [pBox, aBox] = await Promise.all([phone.boundingBox(), address.boundingBox()]);
      expect(pBox && aBox).toBeTruthy();
      expect(aBox?.x).toBeGreaterThan(pBox?.x + pBox?.width);
      // Same grid row: tops align (alignItems: 'start').
      expect(aBox?.y).toBeCloseTo(pBox?.y, 0);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('stays a single column at 1280px and 1024px', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      phones: [{ type: 'mobile', value: '+1 555-0101' }],
      emails: [{ type: 'home', value: 'alice@example.com' }],
    });

    try {
      for (const width of [1280, 1024]) {
        await page.setViewportSize({ width, height: 900 });
        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        const phone = fieldRow(page, 'Phone');
        const address = fieldRow(page, 'Address');
        const [pBox, aBox] = await Promise.all([phone.boundingBox(), address.boundingBox()]);
        expect(pBox && aBox).toBeTruthy();

        if (width === 1280) {
          // lg (1200) is crossed at 1280 — two columns, same as 1440.
          expect(aBox?.x).toBeGreaterThan(pBox?.x + pBox?.width);
        } else {
          // 1024 is below lg — single column, rows stacked at the same left edge.
          expect(aBox?.x).toBeCloseTo(pBox?.x, 0);
          expect(aBox?.y).toBeGreaterThan(pBox?.y);

          // Below lg the field grid is a single column, so a field row spans
          // the full card width. T109 moved the edit pencil next to the field
          // name, so the old "action cluster sits far from the value" proxy for
          // single-column no longer holds — the row width is the faithful guard
          // that the grid doesn't leak below lg. Numbers at 1024: the 256px
          // drawer leaves a 768px content column, so a full row is ~672px while
          // a half-column row would be ~330px — 500 sits cleanly between.
          const rowBox = await phone.boundingBox();
          expect(rowBox, 'phone field row must be present').toBeTruthy();
          expect(rowBox?.width).toBeGreaterThan(500);
        }
      }
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('multi-line / wide fields still span the full card width (not half a column)', async ({
    page,
  }) => {
    // Card notes is the remaining full-span editor in ContactInformation
    // (work_information moved to its own Work section in #1729). It is
    // opt-in, so enable it for this worker's user and restore the defaults.
    const enableResp = await page.request.patch(`${API_BASE_URL}/users/enabled-contact-fields`, {
      data: { fields: [...DEFAULT_ENABLED_CONTACT_FIELDS, 'cardNotes'] },
    });
    expect(enableResp.ok()).toBeTruthy();

    const contact = await createTestContact(page.request, {
      phones: [{ type: 'mobile', value: '+1 555-0101' }],
    });

    try {
      // createTestContact takes the flat form shape, which has no card-notes
      // input; persist one directly through the record PUT instead.
      const read = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      const readBody = await read.json();
      const record = readBody.contact ?? readBody;
      const save = await page.request.put(`${API_BASE_URL}/contacts/${contact.ID}`, {
        data: {
          gender: record.gender ?? '',
          card: {
            ...(record.card ?? {}),
            notes: [{ note: 'Lorem ipsum dolor sit amet, consectetur adipiscing elit.' }],
          },
          crm: record.crm ?? {},
        },
      });
      expect(save.ok(), await save.text()).toBeTruthy();

      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // Card notes is a fullWidth EditableArrayField (T74): it must span the
      // whole card, wider than a half-column field in the same grid.
      const cardNotes = fieldRow(page, 'Notes (vCard)');
      const phone = fieldRow(page, 'Phone');
      const [nBox, pBox] = await Promise.all([cardNotes.boundingBox(), phone.boundingBox()]);
      expect(nBox && pBox).toBeTruthy();
      expect(nBox?.width).toBeGreaterThan(pBox?.width * 1.5);
    } finally {
      await deleteTestContact(page.request, contact.ID);
      await page.request
        .patch(`${API_BASE_URL}/users/enabled-contact-fields`, {
          data: { fields: DEFAULT_ENABLED_CONTACT_FIELDS },
        })
        .catch(() => {});
    }
  });
});

// Two PanelCard headings share a grid row when one sits to the right of the
// other and their tops align. The two title rows often differ (one has an
// action button that vertically centers its heading ~1.4px lower), so allow a
// small tolerance — a stacked card would be hundreds of px apart.
async function expectSideBySide(page: Page, leftLabel: string, rightLabel: string) {
  const left = page.getByRole('heading', { name: leftLabel, exact: true });
  const right = page.getByRole('heading', { name: rightLabel, exact: true });
  await expect(left).toBeVisible();
  await expect(right).toBeVisible();
  const [lBox, rBox] = await Promise.all([left.boundingBox(), right.boundingBox()]);
  expect(lBox && rBox, `${leftLabel} and ${rightLabel} both on screen`).toBeTruthy();
  expect(rBox?.x).toBeGreaterThan(lBox?.x + lBox?.width);
  expect(Math.abs(rBox?.y - lBox?.y)).toBeLessThan(8);
}

async function expectStacked(page: Page, topLabel: string, bottomLabel: string) {
  const top = page.getByRole('heading', { name: topLabel, exact: true });
  const bottom = page.getByRole('heading', { name: bottomLabel, exact: true });
  const [tBox, bBox] = await Promise.all([top.boundingBox(), bottom.boundingBox()]);
  expect(tBox && bBox, `${topLabel} and ${bottomLabel} both on screen`).toBeTruthy();
  expect(bBox?.x).toBeCloseTo(tBox?.x, 0);
  expect(bBox?.y).toBeGreaterThan(tBox?.y);
}

test.describe('T74 section cards — Level 2, 2-up PanelCards', () => {
  test('Cadence and Reminders panels sit side by side at lg+', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await expectSideBySide(page, 'Cadence', 'Reminders');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('Relationships and Connections sit side by side as equal half-columns (people)', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // Testing-round feedback: Connections is a list (T10's ego-centric chain
      // panel), not the force-graph, so it shares the "people" section's second
      // column with Relationships instead of taking a full-width row of its own.
      await expectSideBySide(page, 'Relationships', 'Connections');
      const relationships = page.getByRole('heading', { name: 'Relationships', exact: true });
      const connections = page.getByRole('heading', { name: 'Connections', exact: true });
      const card = (heading: Locator) => heading.locator('..').locator('..').locator('..');
      const [relBox, conBox] = await Promise.all([
        card(relationships).boundingBox(),
        card(connections).boundingBox(),
      ]);
      expect(relBox && conBox).toBeTruthy();
      // Both are half columns (~556px each) — equal width, not the old
      // full-width Connections (~1168px) the pre-issue-8 layout produced.
      expect(Math.abs(conBox?.width - relBox?.width)).toBeLessThan(40);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('Life Events and Conversation Agenda sit side by side at lg+ (timeline)', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await expectSideBySide(page, 'Life Events', 'Conversation Agenda');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('Cadence and Reminders panels stack in a single column below lg', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.setViewportSize({ width: 1024, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await expectStacked(page, 'Cadence', 'Reminders');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('single-card sections (External Links) still span full width', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const externalLinks = page.getByRole('heading', { name: 'External Links', exact: true });
      await expect(externalLinks).toBeVisible();
      const pageWidth = await page.evaluate(() => window.innerWidth);
      // The heading only spans its own text; measure the PanelCard's Card
      // (MuiPaper) it lives in instead.
      const card = externalLinks.locator('..').locator('..').locator('..');
      const box = await card.boundingBox();
      expect(box).toBeTruthy();
      // A non-fullWidth card would be confined to one ~570px column; it must
      // span the section's full ~1168px width.
      expect(box?.width).toBeGreaterThan(900);
      expect(box?.width).toBeLessThan(pageWidth);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});

test.describe('T74 regression guards', () => {
  test.describe('at 390px phone width', () => {
    test.use({ viewport: { width: 390, height: 844 } });

    test('no horizontal overflow, single field column', async ({ page }) => {
      const contact = await createTestContact(page.request, {
        phones: [{ type: 'mobile', value: '+1 555-0101' }],
      });

      try {
        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth > window.innerWidth,
        );
        expect(overflow).toBe(false);

        // The field row must be single column (one caption left-aligned at the
        // card edge, no second column to its right).
        const phone = fieldRow(page, 'Phone');
        const pBox = await phone.boundingBox();
        expect(pBox).toBeTruthy();
        expect(pBox?.x).toBeLessThan(60);
      } finally {
        await deleteTestContact(page.request, contact.ID);
      }
    });
  });

  test('jump-nav still scrolls a section into view with the section grids in place', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const nav = page.getByRole('navigation', { name: /jump to section/i });
      await nav.getByRole('link', { name: 'Cadence & follow-up', exact: true }).click();

      await expect(page).toHaveURL(/#cadence$/);
      // The anchor landing must clear the sticky nav — the section heading's
      // scrollMarginTop (112px) keeps it below the AppBar + jump nav.
      const cadence = page.getByRole('heading', { name: 'Cadence', exact: true });
      await expect(cadence).toBeVisible();
      const top = await cadence.evaluate((el) => el.getBoundingClientRect().top);
      expect(top).toBeGreaterThanOrEqual(100);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
