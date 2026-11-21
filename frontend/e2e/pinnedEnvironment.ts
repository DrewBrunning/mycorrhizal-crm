// e2e/pinnedEnvironment.ts
//
// The pinned rendering environment shared by specs that assert on *measured*
// text layout rather than on behaviour: `visual.spec.ts` (pixel baselines) and
// `pseudoLocale.spec.ts` (scrollWidth-vs-clientWidth overflow under the
// expanded `qps` pseudo-locale).
//
// Both need the same guarantee, for the same reason: the app's type stack is a
// *system* font stack (IBM Plex Sans -> Fira Sans -> ... -> sans-serif, see
// src/theme.ts), so metrics depend on which fonts the host has installed. A
// screenshot baseline generated with IBM Plex Sans installed would differ from
// CI's DejaVu Sans fallback on every glyph, and an overflow assertion would
// pass on one runner and fail on another purely from font metrics. Every test
// therefore registers the app's own primary family (IBM Plex Sans, latin
// subset, OFL-licensed) as a committed @font-face served by a route
// interception (fixtures/fonts/), so identical glyph metrics render on every
// host. The font must actually load before measuring — asserted, and waited
// on, so a broken route fails with a clear message instead of a confusing diff
// or an unexplained pass.
//
// The clock is frozen too: dates the UI renders (birthday ages, reminder dates,
// a dialog's default date) shift layout and text, so a baseline can expire and
// an overflow scan can change on a different day.
//
// This lives outside visual.spec.ts (which first grew the harness) so a second
// spec can reuse it without importing a spec file — importing one would
// register its tests again in the importer.

import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import type { Page } from '@playwright/test';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// All UI-visible dates are chosen relative to this instant so age text,
// reminder dates and dialog default dates render identically forever.
export const FROZEN_TIME = new Date('2026-08-15T12:00:00Z');

// The app's aria-live announcer (App.tsx #211) is visually hidden with
// `clip: inset(50%)`, but its layout box (a full viewport tall) still counts
// toward document height, so a fullPage screenshot would be padded by a big
// blank region. Normalize it to a 1px fixed box for the capture — it stays in
// the a11y tree, it just stops inflating the shot. Harmless for the overflow
// scan, and keeping one stylesheet means the two specs measure the same page.
export const PINNED_CSS = `
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
 * @font-face is registered (and the font fetched) before measuring.
 */
export async function pinRenderingEnvironment(page: Page): Promise<void> {
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
export async function awaitPinFonts(page: Page): Promise<void> {
  await page.addStyleTag({ content: PINNED_CSS });
  await page.evaluate(() => document.fonts.ready);
  const loaded = await page.evaluate(() => document.fonts.check('16px "IBM Plex Sans"'));
  if (!loaded) {
    throw new Error(
      'the pinned IBM Plex Sans webfont must load before measuring layout — check ' +
        'that the __visual_fonts__ route is registered before page.goto()',
    );
  }
}
