import { expect, test, waitForLoading } from './fixtures';

// Issue #1784: the dashboard switched to four columns at `md` (900px) while the
// 256px permanent sidebar was already showing, leaving ~150px per column and a
// 30px horizontal page scroll (more in German). Columns are now 1 / 2 / 4 at
// <600 / 600-1399 / >=1400px, built on minmax(0, 1fr).
const WIDTHS: Array<[number, number]> = [
  [320, 1],
  [599, 1],
  [600, 2],
  [768, 2],
  [899, 2],
  [900, 2],
  [1024, 2],
  [1199, 2],
  [1399, 2],
  [1400, 4],
  [1536, 4],
  [1920, 4],
  [3440, 4],
];

for (const lng of ['en', 'de', 'fr']) {
  test.describe(`Dashboard columns (#1784) [${lng}]`, () => {
    test.beforeEach(async ({ page }) => {
      await page.addInitScript((l) => localStorage.setItem('i18nextLng', l), lng);
    });

    for (const [width, columns] of WIDTHS) {
      test(`${width}px: ${columns} column(s), no horizontal overflow`, async ({ page }) => {
        await page.setViewportSize({ width, height: 900 });
        await page.goto('/');
        await waitForLoading(page);
        await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

        const result = await page.evaluate(() => {
          const grids = Array.from(document.querySelectorAll('main *')).filter(
            (el) => getComputedStyle(el).display === 'grid' && el.querySelector('h2'),
          );
          const grid = grids[grids.length - 1] as HTMLElement | undefined;
          return {
            overflow: document.documentElement.scrollWidth > window.innerWidth,
            columns: grid ? getComputedStyle(grid).gridTemplateColumns.split(' ').length : -1,
            colWidth: grid
              ? Number.parseFloat(getComputedStyle(grid).gridTemplateColumns.split(' ')[0])
              : 0,
          };
        });
        expect(result.overflow, 'the page must not scroll horizontally').toBe(false);
        expect(result.columns).toBe(columns);
        if (width >= 600) expect(result.colWidth).toBeGreaterThanOrEqual(240);
      });
    }
  });
}
