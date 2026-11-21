// e2e/pseudoLocale.spec.ts
//
// The rendered-UI half of the pseudo-localization pass (issue #2536). The
// `qps` mechanism (src/i18n/pseudo.ts, wired in src/i18n/config.ts) exists to
// make two classes of defect visible in a real browser that no key-parity test
// can see:
//
//  * a hardcoded string, which has no key at all, so `locales.test.ts` is
//    structurally blind to it. Every translated leaf is wrapped in ⟦…⟧ and
//    accented, so any un-accented ASCII run of 3+ letters on screen is a
//    literal that bypassed `t()`.
//  * text expansion: the generator pads every string by 40%, roughly what
//    German/French need over English, so a button/chip/table-header/dialog
//    label that no longer fits is caught before a translator reports it.
//
// This spec drives the shipped artifact like every other e2e spec. It sets
// `i18nextLng = 'qps'` in localStorage before the app bundle runs (that is the
// only switch `config.ts` reads), visits the five surfaces the issue names at
// both a desktop and a narrow viewport (issue #2874 — expansion that fits at
// 1280px can still overflow at 600px), and asserts both properties on each.
//
// Two things make the assertions deterministic rather than probabilistic:
//
//  1. Fonts and clock are pinned by `pinnedEnvironment.ts` (shared with
//     visual.spec.ts) — a committed IBM Plex Sans woff2 served by route
//     interception. Without it, `scrollWidth > clientWidth` varies with
//     whatever fallback font the runner has installed, which is the same
//     failure mode the screenshot baselines already had to solve.
//  2. Two markup contracts mark text that is legitimately not translated, so
//     the 3+ letter rule can stay strict (see `isExempt` below for the full
//     reasoning and the MUI built-in case):
//       * `data-user-content` — values owned by the data layer: user-entered
//         records (contact names, emails, phones, addresses, notes, reminder
//         messages), server-provided values (link field type names/protocols)
//         and environment-derived descriptions (user-agent, build version).
//       * `translate="no"` — literal machine values such as example URLs in a
//         placeholder (the standard HTML attribute for "do not translate").
//     A render site for user content without the marker fails this spec, so the
//     exemption cannot silently rot.
//
// The scan reads text nodes, not `innerText`: a translated string and an
// adjacent user-supplied name can share a parent, and a text-node walk sees
// them separately. It also collects `aria-label`/`placeholder`/`title`/`alt`,
// which an innerText-only scan cannot see at all.

import type { Page } from '@playwright/test';
import en from '../src/i18n/locales/en.json' with { type: 'json' };
import { pseudoLocalize } from '../src/i18n/pseudo';
import {
  createTestContact,
  deleteTestContact,
  expect,
  stableClick,
  test,
  waitForLoading,
} from './fixtures';
import { awaitPinFonts, pinRenderingEnvironment } from './pinnedEnvironment';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** The exact string a `t()` key renders under `qps` — for locating controls. */
function pseudo(key: string): string {
  let node: unknown = en;
  for (const part of key.split('.')) {
    if (node === null || typeof node !== 'object') break;
    node = (node as Record<string, unknown>)[part];
  }
  if (typeof node !== 'string') throw new Error(`en.json has no string leaf at ${key}`);
  return pseudoLocalize(node);
}

/** Sets the pseudo-locale switch before the app bundle runs, then pins fonts/clock. */
async function enterPseudoLocale(page: Page): Promise<void> {
  await page.addInitScript(() => localStorage.setItem('i18nextLng', 'qps'));
  // The account language wins over the detected one on load (#2685): the
  // worker user's account is English, so without this the app switches itself
  // straight back to English and the spec would scan an ordinary render. Report
  // `qps` as the account language instead — `applyAccountLanguage` deliberately
  // ignores a language this build cannot render, so the detected `qps` stays.
  // (Same technique as mobileLayout.spec.ts's locale sweep.)
  await page.route('**/api/v1/users/me', async (route) => {
    const response = await route.fetch();
    const body = (await response.json()) as Record<string, unknown>;
    await route.fulfill({ response, json: { ...body, language: 'qps' } });
  });
  await pinRenderingEnvironment(page);
}

/**
 * A hardcoded string or an overflowing element found on the page. Kept small
 * and JSON-serializable so a failure message is a readable inventory instead
 * of a Playwright strict-mode dump.
 */
interface Offence {
  kind: 'text' | 'attribute' | 'overflow';
  /** Where it was found: tag/id/Mui classes, or `tag[attr]`. */
  where: string;
  /** The offending text, or the element's text plus its widths. */
  detail: string;
}

/**
 * Waits until layout has stopped moving: fonts loaded, finite animations
 * settled, two frames painted. A scan run mid-transition would measure a
 * partially-animated width, which is exactly the intermittent failure the
 * spec must not have.
 */
async function settleLayout(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        const finite = document
          .getAnimations()
          .filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
          .map((a) => a.finished.catch(() => {}));
        const capped = Promise.race([Promise.all(finite), new Promise((r) => setTimeout(r, 500))]);
        void capped.then(() => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
      }),
  );
}

/**
 * Runs both scans in the page. Everything is computed in one evaluate so React
 * cannot re-render between the text scan and the overflow scan.
 */
async function scanPage(page: Page): Promise<Offence[]> {
  return page.evaluate(() => {
    const offences: Offence[] = [];
    const ASCII_RUN = /[A-Za-z]{3,}/;

    const describe = (el: Element): string => {
      const classes = [...el.classList]
        .filter((c) => c.startsWith('Mui'))
        .slice(0, 2)
        .map((c) => `.${c}`)
        .join('');
      return `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ''}${classes}`;
    };

    const isVisible = (el: Element): boolean => {
      if (el.checkVisibility && !el.checkVisibility({ opacityProperty: false })) return false;
      const rect = el.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0;
    };

    // Two markers keep the 3+ letter rule strict without false positives:
    //  * [data-user-content] — values owned by the data layer: user-entered
    //    records, server-provided names/templates, environment-derived
    //    descriptions (user-agent, build version). Never translated.
    //  * [translate="no"] — literal machine values such as example URLs in a
    //    placeholder. The HTML standard defines this attribute as "do not
    //    translate", so translation tooling honours it too.
    // MUI's own built-in control labels (Autocomplete's Clear/Open, ...) come
    // from @mui/material/locale via muiLocaleFor(), which deliberately has no
    // `qps` entry (part 1 of #2536): they are not i18next keys and cannot be
    // pseudo-localized, so they are exempt here rather than reported as app
    // copy. If another MUI control string appears on a surface, this selector
    // list is where it belongs.
    const MUI_BUILTIN_LABELS = '.MuiAutocomplete-endAdornment';
    const isExempt = (el: Element | null): boolean =>
      !!el?.closest(`[data-user-content], [translate="no"], ${MUI_BUILTIN_LABELS}`);

    /**
     * Drops everything inside ⟦…⟧, tracking nesting: a translated template can
     * interpolate another translated string (`Copy {{label}}` with a localized
     * field name) plus a user value, so the brackets nest. A per-node balanced
     * walk is what makes "outside the brackets" mean what it says.
     */
    const stripPseudo = (value: string): string => {
      let outside = '';
      let depth = 0;
      for (const ch of value) {
        if (ch === '⟦') depth++;
        else if (ch === '⟧') depth = Math.max(0, depth - 1);
        else if (depth === 0) outside += ch;
      }
      return outside;
    };

    /** Text outside `⟦…⟧` with a 3+ letter ASCII run is a hardcoded literal. */
    const hardcodedPart = (value: string): string | null => {
      const outside = stripPseudo(value);
      return ASCII_RUN.test(outside) ? outside.trim() : null;
    };

    // --- visible text -----------------------------------------------------
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const text = node.nodeValue ?? '';
      if (!text.trim()) continue;
      const parent = node.parentElement;
      if (!parent) continue;
      if (isExempt(parent)) continue;
      if (['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'TEXTAREA'].includes(parent.tagName)) {
        continue;
      }
      // A text node with no client rect is not rendered (display:none and
      // friends); measuring instead of trusting the style cascade keeps this
      // correct for MUI's visibility helpers too.
      const range = document.createRange();
      range.selectNodeContents(node);
      if (range.getClientRects().length === 0) continue;
      range.detach();
      const hardcoded = hardcodedPart(text);
      if (hardcoded) {
        offences.push({
          kind: 'text',
          where: describe(parent),
          detail: text.trim().slice(0, 160),
        });
      }
    }

    // --- attributes -------------------------------------------------------
    // aria-label / placeholder / title / alt are rendered by assistive tech,
    // the tooltip layer and the browser, not by innerText — so they need their
    // own collection step or a hardcoded one stays invisible to this spec.
    for (const el of document.querySelectorAll('[aria-label], [placeholder], [title], [alt]')) {
      if (isExempt(el) || !isVisible(el)) continue;
      for (const attr of ['aria-label', 'placeholder', 'title', 'alt']) {
        const value = el.getAttribute(attr);
        if (!value) continue;
        const hardcoded = hardcodedPart(value);
        if (!hardcoded) continue;
        offences.push({
          kind: 'attribute',
          where: `${describe(el)}[${attr}]`,
          detail: value.trim().slice(0, 160),
        });
      }
    }

    // --- overflow ---------------------------------------------------------
    // User content can be any length by definition, so an overflowing chip
    // whose text the user typed is not evidence of a translation-expansion
    // bug (and would make this spec depend on which contact the list shows).
    for (const el of document.querySelectorAll('button, [role="tab"], th, .MuiChip-label')) {
      if (isExempt(el) || !isVisible(el)) continue;
      const style = getComputedStyle(el);
      if (style.textOverflow === 'ellipsis') continue;
      if (el.clientWidth === 0) continue;
      if (el.scrollWidth > el.clientWidth + 1) {
        offences.push({
          kind: 'overflow',
          where: describe(el),
          detail: `"${(el.textContent ?? '').trim().slice(0, 100)}" ${el.scrollWidth}>${el.clientWidth}`,
        });
      }
    }

    return offences;
  });
}

/**
 * Proves the pseudo-locale actually applied. Every translated string is
 * bracket-wrapped, and no English source string contains ⟦, so the app shell's
 * copy (nav, skip link, headings) is a reliable probe on every surface — unlike
 * an <html lang> check (config.ts reports `en` for `qps` so axe's
 * html-lang-valid rule stays green) or a heading check (the contact detail's h1
 * is the contact's name, i.e. user content).
 */
async function expectPseudoLocaleActive(page: Page): Promise<void> {
  await expect(page.locator('body')).toContainText('⟦');
}

/** Asserts the surface is fully localised and nothing overflows. */
async function expectPseudoClean(page: Page, surface: string): Promise<void> {
  await settleLayout(page);
  const offences = await scanPage(page);
  expect(
    offences,
    `${surface} under the qps pseudo-locale:\n` +
      'text/attribute offences are hardcoded strings that need t(); ' +
      'overflow offences need a layout fix or an explicit text-overflow: ellipsis.\n' +
      'User-supplied content must be rendered inside [data-user-content].\n' +
      JSON.stringify(offences, null, 2),
  ).toEqual([]);
}

// Two viewports, one assertion body (issue #2874). `qps` pads every translated
// string by 40% — roughly the German/French expansion the pass exists to
// simulate — and a label that fits at 1280px can still overflow on a narrow
// screen, which a single pinned width is structurally blind to.
//
// * desktop 1280x720 matches visual.spec.ts's baselines and the layout a
//   translator's screenshots would come from.
// * narrow 600x800 is the narrowest width that still renders the shell's
//   *text-bearing* AppBar. `compactNav` is `theme.breakpoints.down('sm')`,
//   i.e. strictly below 600px, so at 375px the labelled Logout button moves
//   into the account menu and the pass would never see the regression this
//   issue reports. 600px is where the desktop-style toolbar is at its
//   tightest, so it is the worst case for that layout (and it reproduces the
//   113px-in-111px Logout overflow).
const VIEWPORTS: ReadonlyArray<{
  label: string;
  viewport: { width: number; height: number };
}> = [
  { label: 'desktop', viewport: { width: 1280, height: 720 } },
  { label: 'narrow', viewport: { width: 600, height: 800 } },
];

// Layout metrics are engine-sensitive: the pinned webfont fixes the glyphs,
// but each engine still measures and rounds text its own way, so the same
// element can sit a pixel over the line on Firefox. visual.spec.ts pins its
// shots to Chromium for the same reason (#1479); the nightly all-browsers run
// must not read an engine difference as a translation bug.
test.beforeEach(({ browserName }) => {
  test.skip(
    browserName !== 'chromium',
    'pseudo-locale layout metrics are Chromium-pinned (see #1479)',
  );
});

// ---------------------------------------------------------------------------
// Surfaces
// ---------------------------------------------------------------------------

/** Navigates, pins fonts and waits for the loading overlay — every surface. */
async function openPinned(page: Page, path: string): Promise<void> {
  await page.goto(path);
  await awaitPinFonts(page);
  await waitForLoading(page);
}

const noCleanup = async (): Promise<void> => {};

/**
 * A surface rendered for the shared assertion body. `prepare` owns any
 * spec-owned fixture and returns the navigation/cleanup pair, so a failure at
 * either the open or the assertion still deletes the fixture.
 */
interface Surface {
  name: string;
  prepare: (page: Page) => Promise<{ open: () => Promise<void>; cleanup: () => Promise<void> }>;
}

const SURFACES: readonly Surface[] = [
  {
    name: 'Dashboard',
    prepare: async (page) => ({
      open: async () => {
        await openPinned(page, '/');
        // The h1 is translated, so locate it structurally rather than by its
        // (now accented) English name.
        await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
      },
      cleanup: noCleanup,
    }),
  },
  {
    name: 'Contacts list',
    prepare: async (page) => ({
      open: async () => {
        await openPinned(page, '/contacts');
        // The seeded sample contact proves the list (and its user-content
        // markers) actually rendered; the name itself is user content, not
        // translatable.
        await expect(page.getByText('Alice Johnson').first()).toBeVisible();
      },
      cleanup: noCleanup,
    }),
  },
  {
    name: 'Contact detail',
    // A spec-owned contact with fixed names: the detail page renders user
    // content (name, email, notes, org) that must be exempted, and a name that
    // varies per run would make the exemption impossible to review.
    prepare: async (page) => {
      const contact = await createTestContact(page.request, {
        firstname: 'E2EFixtureVera',
        lastname: 'Pseudo',
        emails: [{ type: 'home', value: 'vera.pseudo@example.com' }],
        phones: [{ type: 'mobile', value: '+1 555-0199' }],
        birthday: '1988-05-20',
        notes: 'Noted while reviewing the pseudo-locale pass.',
        circles: ['Friends'],
      });
      return {
        open: async () => {
          await openPinned(page, `/contacts/${contact.ID}`);
          await expect(page.getByText('E2EFixtureVera Pseudo').first()).toBeVisible();
        },
        cleanup: () => deleteTestContact(page.request, contact.ID),
      };
    },
  },
  {
    name: 'Settings',
    prepare: async (page) => ({
      open: () => openPinned(page, '/settings'),
      cleanup: noCleanup,
    }),
  },
  {
    name: 'Add reminder dialog',
    prepare: async (page) => {
      const contact = await createTestContact(page.request, {
        firstname: 'E2EFixtureVera',
        lastname: 'Pseudo',
      });
      return {
        open: async () => {
          await openPinned(page, `/contacts/${contact.ID}`);
          // The trigger's accessible name is translated too, so find it by the
          // Reminders panel's translated heading rather than an English regex.
          const remindersCard = page.locator('.MuiCard-root').filter({
            has: page.getByRole('heading', { name: pseudo('reminders.title'), exact: true }),
          });
          await stableClick(remindersCard.getByRole('button', { name: pseudo('reminders.add') }));

          const dialog = page.getByRole('dialog');
          await expect(dialog).toBeVisible();
          await expect(dialog.getByRole('heading')).toContainText(pseudo('reminders.addReminder'));
        },
        cleanup: () => deleteTestContact(page.request, contact.ID),
      };
    },
  },
];

for (const { label, viewport } of VIEWPORTS) {
  test.describe(`Pseudo-localization (qps, ${label} ${viewport.width}px)`, () => {
    test.use({ viewport });

    for (const surface of SURFACES) {
      test(`${surface.name} renders only translated text and does not overflow`, async ({
        page,
      }) => {
        await enterPseudoLocale(page);
        const scene = await surface.prepare(page);
        try {
          await scene.open();
          await expectPseudoLocaleActive(page);
          await expectPseudoClean(page, `${surface.name} (${label} ${viewport.width}px)`);
        } finally {
          await scene.cleanup();
        }
      });
    }
  });
}
