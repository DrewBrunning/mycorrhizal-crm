import type { Page } from '@playwright/test';
import { createTestContact, deleteTestContact, expect, test, waitForLoading } from './fixtures';

// Issue #1782: between 900 and 1199px (permanent sidebar, below `lg`) the
// contact page's jump-nav tab labels were drawn on top of each other and the
// header's action row was clipped at the card edge, leaving Subscribe / Export
// / Archive / Delete unreachable. These specs pin the invariants at the widths
// the issue names plus the 1200px boundary where the wide header takes over.
const WIDTHS = [900, 1000, 1100, 1199, 1200] as const;

interface Box {
  left: number;
  right: number;
  top: number;
  bottom: number;
}

async function navLinkBoxes(page: Page): Promise<Box[]> {
  return page.getByRole('navigation', { name: /jump to section/i }).evaluate((nav) =>
    Array.from(nav.querySelectorAll('a')).map((a) => {
      const r = a.getBoundingClientRect();
      return { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
    }),
  );
}

/** Every visible button in the header card must sit inside the card's box. */
async function clippedHeaderButtons(page: Page): Promise<string[]> {
  return page.getByRole('heading', { level: 1 }).evaluate((h1) => {
    const card = h1.closest('.MuiCard-root');
    if (!card) return ['no header card'];
    const c = card.getBoundingClientRect();
    const clipped: string[] = [];
    for (const b of Array.from(card.querySelectorAll('button'))) {
      const r = b.getBoundingClientRect();
      if (r.width === 0 || r.height === 0) continue;
      if (getComputedStyle(b).opacity === '0') continue; // hover-reveal pencils
      if (r.left < c.left - 1 || r.right > c.right + 1) {
        clipped.push(b.getAttribute('aria-label') ?? b.textContent ?? '?');
      }
    }
    return clipped;
  });
}

for (const width of WIDTHS) {
  test.describe(`Contact page at ${width}px (#1782)`, () => {
    test.use({ viewport: { width, height: 900 } });

    test('jump-nav tabs do not overlap and header actions are not clipped', async ({ page }) => {
      const contact = await createTestContact(page.request);
      try {
        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        const links = await navLinkBoxes(page);
        // Below md the nav is a Select (no links); at md and above it is a strip.
        expect(links.length).toBeGreaterThan(3);
        for (let i = 1; i < links.length; i++) {
          expect(
            links[i].left,
            `tab ${i} starts before tab ${i - 1} ends (overlapping labels)`,
          ).toBeGreaterThanOrEqual(links[i - 1].right - 1);
        }

        expect(await clippedHeaderButtons(page)).toEqual([]);
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth > window.innerWidth,
        );
        expect(overflow, 'the page must not scroll horizontally').toBe(false);
      } finally {
        await deleteTestContact(page.request, contact.ID);
      }
    });

    test('every header action is reachable (inline or in the menu), Delete last', async ({
      page,
    }) => {
      const contact = await createTestContact(page.request);
      try {
        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        // Below lg the single icon button; at lg and above the "More" button.
        const trigger = page.getByRole('button', { name: /^(Actions|More actions)$/ });
        await trigger.focus();
        await expect(trigger).toBeFocused();
        await page.keyboard.press('Enter');

        const menu = page.getByRole('menu');
        await expect(menu).toBeVisible();
        for (const name of ['Merge', 'Subscribe via feed', 'Export as vCard 4.0', 'Archive']) {
          await expect(menu.getByRole('menuitem', { name })).toBeVisible();
        }
        const items = menu.getByRole('menuitem');
        await expect(items.last()).toHaveText('Delete Contact');

        // The last item is inside the viewport, i.e. not clipped off-screen.
        const box = await items.last().boundingBox();
        expect(box).not.toBeNull();
        expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(width);
      } finally {
        await deleteTestContact(page.request, contact.ID);
      }
    });
  });
}

test.describe('Contact page name wrapping at 320px (#1782)', () => {
  test.use({ viewport: { width: 320, height: 640 } });

  test('a two-word name wraps between words, not mid-word', async ({ page }) => {
    const contact = await createTestContact(page.request);
    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      const h1 = page.getByRole('heading', { level: 1 });
      const wordBreaks = await h1.evaluate((el) => {
        const cs = getComputedStyle(el);
        return { wordBreak: cs.wordBreak, overflowWrap: cs.overflowWrap };
      });
      expect(wordBreaks.wordBreak).toBe('normal');
      expect(wordBreaks.overflowWrap).not.toBe('anywhere');
      // The avatar stacks above the name below sm.
      const avatar = await page.locator('.MuiAvatar-root').first().boundingBox();
      const heading = await h1.boundingBox();
      expect((avatar?.y ?? 0) + (avatar?.height ?? 0)).toBeLessThanOrEqual(heading?.y ?? 0);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
