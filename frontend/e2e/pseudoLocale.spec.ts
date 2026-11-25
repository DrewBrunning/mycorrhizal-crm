// e2e/pseudoLocale.spec.ts
//
// The rendered-UI half of the pseudo-localization pass (issues #2536, #2920).
// The `qps` mechanism (src/i18n/pseudo.ts, wired in src/i18n/config.ts) exists
// to make defect classes visible in a real browser that no key-parity test can
// see:
//
//  * a hardcoded string, which has no key at all, so `locales.test.ts` is
//    structurally blind to it. Every translated leaf is wrapped in ⟦…⟧ and
//    accented, so any un-accented ASCII run of 3+ letters on screen is a
//    literal that bypassed `t()`.
//  * text expansion: the generator pads every string by 40%, roughly what
//    German/French need over English, so a button/chip/table-header/dialog
//    label that no longer fits is caught before a translator reports it.
//  * server-originated prose: a reason parameter rendered through a
//    `{{reason}}`-only template looks translated to the bracket rule, so the
//    spec also scans the exact strings the API put in `reason_params`
//    (issue #2912) and un-exempts reminder rows that carry an
//    `occasion_obligation_id` (issue #2915).
//  * a hand-assembled month/day: a translated month name followed by a bare
//    day number is invisible to the bracket rule (both halves look localised)
//    but wrong in every language that orders the day first (issue #2914).
//
// Issue #2920 item 4 widens the pass from five fixed surfaces to *every*
// route in src/App.tsx (parsed by scripts/check-e2e-routes.mjs, not copied)
// plus the dialogs the audit found English in, with the worker user seeded
// into the round-2 populated states: an occasion with a reminder, a contact
// with an anniversary shared to the user, a timeline activity with two
// participants and a pending incoming share.
//
// This spec drives the shipped artifact like every other e2e spec. It sets
// `i18nextLng = 'qps'` in localStorage before the app bundle runs (that is the
// only switch `config.ts` reads), visits each surface at both a desktop and a
// narrow viewport (issue #2874 — expansion that fits at 1280px can still
// overflow at 600px), and asserts both properties on each.
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
//         A reminder message that carries an `occasion_obligation_id` is NOT
//         exempt: its "Occasion — " half is server copy, not user input
//         (#2915).
//       * `translate="no"` — literal machine values such as example URLs in a
//         placeholder (the standard HTML attribute for "do not translate").
//     A render site for user content without the marker fails this spec, so the
//     exemption cannot silently rot.
//
// The scan reads text nodes, not `innerText`: a translated string and an
// adjacent user-supplied name can share a parent, and a text-node walk sees
// them separately. It also collects `aria-label`/`placeholder`/`title`/`alt`,
// which an innerText-only scan cannot see at all.

import type { APIRequestContext, Page } from '@playwright/test';
import { loadRoutes } from '../scripts/check-e2e-routes.mjs';
import en from '../src/i18n/locales/en.json' with { type: 'json' };
import { pseudoLocalize } from '../src/i18n/pseudo';
import {
  test as baseTest,
  createTestContact,
  deleteTestContact,
  deleteThrowawayUser,
  expect,
  LOGGED_OUT,
  makeThrowawayUser,
  SKIP_A11Y_SCAN,
  stableClick,
  waitForLoading,
} from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';
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

/** Escapes a literal for RegExp; used when an accessible name has interpolation. */
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
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
  kind: 'text' | 'attribute' | 'overflow' | 'server-prose' | 'date-order';
  /** Where it was found: tag/id/Mui classes, or `tag[attr]`. */
  where: string;
  /** The offending text, or the element's text plus its widths. */
  detail: string;
}

/** Options for one scan pass, computed in Node from the spec's own fixtures. */
interface ScanOptions {
  /**
   * Text that identifies a `[data-user-content]` node whose content is *not*
   * all user input. The node is still scanned: a reminder created from an
   * occasion stores the server's English "Occasion — " prefix in its message
   * (#2915), so exempting the whole row would hide the defect.
   */
  forceScan?: string[];
  /**
   * Server-originated prose (API `reason_params` values). It is checked even
   * when it sits inside ⟦…⟧, because a `{{reason}}`-only template otherwise
   * makes English server copy look translated (#2912).
   */
  serverProse?: string[];
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

// Every pseudo-localized month name, for the hand-assembled month/day check
// (#2914). The keys are shared with the occasion dialog's month picker.
const PSEUDO_MONTHS = Array.from({ length: 12 }, (_, i) => pseudo(`occasions.months.${i + 1}`));

// `new Intl.DateTimeFormat('qps', …)` falls back to the runner's default
// locale, so a correctly formatted year-less date renders as unaccented
// English ("March 3" or "3 March"). That is machine-produced locale data, not
// app copy; allow it only in the occasions section, where #2914 lives, so the
// hardcoded scan elsewhere stays strict.
const INTL_YEARLESS_MONTH_DAY =
  /^(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2}$|^\d{1,2}\s+(January|February|March|April|May|June|July|August|September|October|November|December)$/;

// A timestamp the app builds from the user's Date/Time-format preferences,
// optionally followed by an `Intl.RelativeTimeFormat` hint:
// `24.11.2026 10:44 PM (in 101 days)`, `Nov 24, 2026 10:44 PM (2 hours ago)`.
// The date half is one of DateFormatProvider's nine formats, the time half is
// the user's hour cycle, and the relative half is machine-generated — none of
// it is translatable copy, so the 3+ letter rule could only ever report `PM`
// and `in … days` here. The *whole* value must match, so a hardcoded sentence
// that merely contains a date is still an offence, and hand-assembled
// month/day labels stay caught by the #2914 check below.
const DATE_TOKENS =
  '(?:\\d{1,2}[./-]\\d{1,2}[./-]\\d{4}|\\d{4}-\\d{2}-\\d{2}|' +
  '(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\\s\\d{1,2},?\\s\\d{4}|' +
  '\\d{1,2}\\s(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*,?\\s\\d{4})';
const INTL_INSTANT = new RegExp(
  // `\\s` rather than a literal space: Intl puts a narrow no-break space before
  // `PM` in some ICU versions.
  `^${DATE_TOKENS}\\s\\d{1,2}:\\d{2}(?::\\d{2})?(?:\\s?[AP]M)?` +
    '(?:\\s\\((?:in\\s\\d+\\s(?:second|minute|hour|day|week|month|quarter|year)s?' +
    '|\\d+\\s(?:second|minute|hour|day|week|month|quarter|year)s?\\sago)\\))?$',
);

/**
 * Runs every scan in the page. Everything is computed in one evaluate so React
 * cannot re-render between the text scan and the overflow scan.
 */
async function scanPage(page: Page, options: ScanOptions = {}): Promise<Offence[]> {
  return page.evaluate(
    ({ forceScan, serverProse, pseudoMonths, intlYearlessMonthDay, intlInstant }) => {
      const offences: Offence[] = [];
      const ASCII_RUN = /[A-Za-z]{3,}/;
      const INTL_YEARLESS = new RegExp(intlYearlessMonthDay);
      const INTL_TIMESTAMP = new RegExp(intlInstant);
      const MONTH_DAY = pseudoMonths.map(
        (month) => new RegExp(`${month.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s+\\d{1,2}(?!\\d)`),
      );

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
      // MUI's Alert close action is a built-in English "Close" label from
      // @mui/material, like the Autocomplete strings above. TablePagination's
      // own chrome ("Rows per page:", "Go to previous page", …) is the same
      // kind of label and arrives with the paginated admin tables, so the whole
      // widget is exempt via its root class (the child slots carry
      // MuiTablePagination-* names, which `.MuiTablePagination` does not match).
      const MUI_BUILTIN_LABELS =
        '.MuiAutocomplete-endAdornment, .MuiAlert-action, .MuiTablePagination-root';
      // MapLibre's own controls and canvas labels ("MapLibre", "Map", "Toggle
      // attribution") are third-party widget chrome, not this app's copy.
      const THIRD_PARTY_MAP_LABELS = '.maplibregl-map';
      const isForceScanned = (value: string): boolean =>
        forceScan.some((needle) => needle.length > 0 && value.includes(needle));
      const isExempt = (el: Element | null, value = ''): boolean =>
        !!el?.closest(
          `[data-user-content], [translate="no"], ${MUI_BUILTIN_LABELS}, ${THIRD_PARTY_MAP_LABELS}`,
        ) && !isForceScanned(value);

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

      /** A translated month name followed by a bare day is a hand-built date (#2914). */
      const handBuiltMonthDay = (value: string): string | null => {
        for (const re of MONTH_DAY) {
          const match = value.match(re);
          if (match) return match[0];
        }
        return null;
      };

      /** Server prose that must not reach the UI untranslated (#2912). */
      const serverProseHit = (value: string): string | null => {
        for (const prose of serverProse) {
          if (prose.length > 0 && value.includes(prose) && ASCII_RUN.test(prose)) return prose;
        }
        return null;
      };

      const checkValue = (el: Element, value: string, where: string): void => {
        // An instant rendered by Intl carries no translatable copy — see
        // INTL_INSTANT above.
        if (INTL_TIMESTAMP.test(value.trim())) {
          return;
        }
        const prose = serverProseHit(value);
        if (prose) {
          offences.push({ kind: 'server-prose', where, detail: prose.slice(0, 160) });
        }
        const monthDay = handBuiltMonthDay(value);
        if (monthDay) {
          offences.push({ kind: 'date-order', where, detail: monthDay });
        }
        if (el.closest('#occasions') && INTL_YEARLESS.test(value.trim())) {
          return;
        }
        const hardcoded = hardcodedPart(value);
        if (hardcoded) {
          offences.push({ kind: 'text', where, detail: hardcoded.slice(0, 160) });
        }
      };

      // --- visible text -----------------------------------------------------
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        const text = node.nodeValue ?? '';
        if (!text.trim()) continue;
        const parent = node.parentElement;
        if (!parent) continue;
        if (isExempt(parent, text)) continue;
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
        checkValue(parent, text, describe(parent));
      }

      // --- attributes -------------------------------------------------------
      // aria-label / placeholder / title / alt are rendered by assistive tech,
      // the tooltip layer and the browser, not by innerText — so they need their
      // own collection step or a hardcoded one stays invisible to this spec.
      for (const el of document.querySelectorAll('[aria-label], [placeholder], [title], [alt]')) {
        if (!isVisible(el)) continue;
        for (const attr of ['aria-label', 'placeholder', 'title', 'alt']) {
          const value = el.getAttribute(attr);
          if (!value) continue;
          if (isExempt(el, value)) continue;
          checkValue(el, value, `${describe(el)}[${attr}]`);
        }
      }

      // --- overflow ---------------------------------------------------------
      // User content can be any length by definition, so an overflowing chip
      // whose text the user typed is not evidence of a translation-expansion
      // bug (and would make this spec depend on which contact the list shows).
      for (const el of document.querySelectorAll('button, [role="tab"], th, .MuiChip-label')) {
        if (isExempt(el, el.textContent ?? '') || !isVisible(el)) continue;
        // A count or other machine token ("1", "12", "3/4") cannot show text
        // expansion. MUI also paints a badge *outside* the button box, which
        // then reads as a few pixels of scrollWidth on the button that carries
        // it; that is the #2906 target-size question, not a translation bug.
        if (!/[A-Za-zÀ-ÿ]/.test(el.textContent ?? '')) continue;
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
    },
    {
      forceScan: options.forceScan ?? [],
      serverProse: options.serverProse ?? [],
      pseudoMonths: PSEUDO_MONTHS,
      intlYearlessMonthDay: INTL_YEARLESS_MONTH_DAY.source,
      intlInstant: INTL_INSTANT.source,
    },
  );
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

interface ScanRecord {
  label: string;
  offences: Offence[];
}

type RecordScan = (label: string, options?: ScanOptions) => Promise<void>;

/**
 * A counted allowlist entry for a known, already-recorded offender: the exact
 * offence (kind + element + text) and how many times a surface shows it. It is
 * a ratchet — a count that no longer matches fails as stale, so the entry must
 * be deleted when the layout/translation is fixed (same shape as
 * scripts/check-e2e-routes.mjs's exemption file).
 */
interface KnownOffence {
  kind: Offence['kind'];
  where: string;
  detail: string;
  count: number;
  reason: string;
}

const offenceKey = (o: Pick<Offence, 'kind' | 'where' | 'detail'>): string =>
  `${o.kind}\u0000${o.where}\u0000${o.detail}`;

/**
 * `/settings/data` at 600px: the four Import buttons are one non-wrapping row,
 * so the 40%-expanded labels (German/French worst case) are 2–17px wider than
 * their buttons. Tracked as a follow-up layout fix outside #2920's test scope;
 * delete these entries when the row wraps.
 */
const DATA_SETTINGS_OVERFLOWS: readonly KnownOffence[] = [
  {
    kind: 'overflow',
    where: 'button.MuiButtonBase-root.MuiButton-root',
    detail: '"⟦Ímpórt Cóntácts~~~~~~⟧" 127>110',
    count: 1,
    reason: 'Data Import buttons are a non-wrapping row at 600px; follow-up layout fix.',
  },
  {
    kind: 'overflow',
    where: 'button.MuiButtonBase-root.MuiButton-root',
    detail: '"⟦Ímpórt fróm Mónícá~~~~~~~~⟧" 118>115',
    count: 1,
    reason: 'Data Import buttons are a non-wrapping row at 600px; follow-up layout fix.',
  },
  {
    kind: 'overflow',
    where: 'button.MuiButtonBase-root.MuiButton-root',
    detail: '"⟦Ímpórt fróm Méérkát~~~~~~~~⟧" 125>120',
    count: 1,
    reason: 'Data Import buttons are a non-wrapping row at 600px; follow-up layout fix.',
  },
];

/**
 * The CSV import mapping table renders its target fields from
 * `CONTACT_FIELD_LABELS` (src/api/import.ts), English constants rather than
 * i18next keys. That is a real follow-up i18n gap outside #2920's test scope,
 * so the three labels this fixture maps are pinned exactly; delete an entry
 * when the wizard starts translating them.
 */
const IMPORT_FIELD_LABEL_OFFENCES: readonly KnownOffence[] = [
  {
    kind: 'text',
    where: 'div.MuiSelect-select.MuiSelect-outlined',
    detail: 'First Name',
    count: 1,
    reason: 'Import mapping renders CONTACT_FIELD_LABELS (English constant); follow-up i18n.',
  },
  {
    kind: 'text',
    where: 'div.MuiSelect-select.MuiSelect-outlined',
    detail: 'Last Name',
    count: 1,
    reason: 'Import mapping renders CONTACT_FIELD_LABELS (English constant); follow-up i18n.',
  },
  {
    kind: 'text',
    where: 'div.MuiSelect-select.MuiSelect-outlined',
    detail: 'Email',
    count: 1,
    reason: 'Import mapping renders CONTACT_FIELD_LABELS (English constant); follow-up i18n.',
  },
];

/** Asserts a scene's collected scans are clean; one failure lists every step. */
function expectScansClean(
  scans: ScanRecord[],
  surface: string,
  allowed: readonly KnownOffence[] = [],
): void {
  const allowedByKey = new Map(allowed.map((entry) => [offenceKey(entry), entry]));
  const seen = new Map<string, number>();
  const failing: ScanRecord[] = [];
  for (const scan of scans) {
    const offences = scan.offences.filter((offence) => {
      const key = offenceKey(offence);
      if (!allowedByKey.has(key)) return true;
      seen.set(key, (seen.get(key) ?? 0) + 1);
      return false;
    });
    if (offences.length > 0) failing.push({ label: scan.label, offences });
  }

  const stale = allowed.filter((entry) => (seen.get(offenceKey(entry)) ?? 0) !== entry.count);
  expect(
    stale.map((entry) => ({ key: offenceKey(entry), reason: entry.reason })),
    `${surface}: a known-offence allowlist entry no longer matches (fixed or reworded?) -- ` +
      'delete or correct it instead of widening the allowlist.',
  ).toEqual([]);

  expect(
    failing,
    `${surface} under the qps pseudo-locale:\n` +
      'text/attribute offences are hardcoded or server-originated strings that need t(); ' +
      'overflow offences need a layout fix or an explicit text-overflow: ellipsis; ' +
      'date-order offences are hand-assembled month/day labels (#2914).\n' +
      'User-supplied content must be rendered inside [data-user-content].\n' +
      JSON.stringify(failing, null, 2),
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

// ---------------------------------------------------------------------------
// Populated worker state (#2920 item 4)
// ---------------------------------------------------------------------------

interface PseudoSeed {
  contactId: number;
  contactName: string;
  activityTitle: string;
  occasionLabel: string;
  entityName: string;
  entityId: string;
  /** `label: contact name` — the user-owned half of the occasion reminder (#2915). */
  occasionReminderText: string;
  /** The accepted share's label (the sender's contact name). */
  acceptedShareLabel: string;
  /** The pending incoming share's label. */
  pendingShareLabel: string;
  /** The worker's own outgoing share label (for the Share edit dialog). */
  outgoingShareLabel: string;
}

async function jsonOrThrow(response: Awaited<ReturnType<APIRequestContext['get']>>, label: string) {
  expect(response.ok(), `${label}: ${response.status()} ${await response.text()}`).toBeTruthy();
  return response.json() as Promise<Record<string, unknown>>;
}

async function postJSON(
  request: APIRequestContext,
  path: string,
  data: unknown,
  label: string,
): Promise<Record<string, unknown>> {
  const response = await request.post(`${API_BASE_URL}${path}`, { data: data as object });
  return jsonOrThrow(response, label);
}

/** Creates one contact and returns it; used for the sender account too. */
async function createContact(request: APIRequestContext, overrides: Record<string, unknown>) {
  return createTestContact(request, { firstname: `${E2E_CONTACT_PREFIX}Pseudo`, ...overrides });
}

async function seedWorkerState(
  browser: import('@playwright/test').Browser,
  storageStatePath: string,
): Promise<{ seed: PseudoSeed; cleanup: () => Promise<void> }> {
  const worker = await browser.newContext({ storageState: storageStatePath });
  const sender = await browser.newContext({ storageState: LOGGED_OUT });
  const senderUser = makeThrowawayUser('pseudo_sender');
  let contactId = 0;
  let secondContactId = 0;
  let duplicateIds: number[] = [];
  let entityId = '';

  try {
    const me = await jsonOrThrow(await worker.request.get(`${API_BASE_URL}/users/me`), 'users/me');
    const workerId = Number(me.ID ?? me.id);

    const main = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoMain`,
      lastname: 'Anchor',
      emails: [{ type: 'home', value: 'pseudo-main@example.com' }],
      birthday: '1988-05-20',
    });
    contactId = main.ID;
    const second = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoSecond`,
      lastname: 'Participant',
    });
    secondContactId = second.ID;

    // Two same-name/same-email contacts make the duplicate review non-empty.
    const dupA = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoDup`,
      lastname: 'Pair',
      emails: [{ type: 'home', value: 'pseudo-dup@example.com' }],
    });
    const dupB = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoDup`,
      lastname: 'Pair',
      emails: [{ type: 'home', value: 'pseudo-dup@example.com' }],
    });
    duplicateIds = [dupA.ID, dupB.ID];

    const entityBody = await postJSON(
      worker.request,
      '/entities',
      { kind: 'organisation', name: 'E2E Pseudo Organisation' },
      'entity create',
    );
    const entity = (entityBody.entity ?? entityBody) as Record<string, unknown>;
    entityId = String(entity.id);

    // An active occasion whose reminder lands inside the dashboard's 30-day
    // window. The backend materializes the reminder row and stores the
    // English "Occasion — " prefix pre-fix (#2915).
    const anchor = new Date(Date.now() + 10 * 24 * 60 * 60 * 1000);
    await postJSON(
      worker.request,
      '/occasion-obligations',
      {
        entity_id: main.uid,
        kind: 'card',
        label: 'E2E pseudo card',
        anchor_month: anchor.getUTCMonth() + 1,
        anchor_day: anchor.getUTCDate(),
        lead_time_days: 1,
      },
      'occasion create',
    );
    const mainName = `${main.firstname} ${main.lastname}`;

    await postJSON(
      worker.request,
      '/activities',
      {
        title: 'E2E pseudo coffee',
        type: 'visit',
        date: new Date().toISOString(),
        contact_ids: [main.ID, second.ID],
      },
      'activity create',
    );

    // A second account owns the shared contact and the pending share. Register
    // through the API, then log in so its request context carries the cookie.
    const registered = await sender.request.post(`${API_BASE_URL}/register`, {
      data: senderUser as unknown as object,
    });
    expect(
      registered.ok(),
      `sender register: ${registered.status()} ${await registered.text()}`,
    ).toBeTruthy();
    const login = await sender.request.post(`${API_BASE_URL}/login`, {
      data: { identifier: senderUser.username, password: senderUser.password },
    });
    expect(login.ok(), `sender login: ${login.status()} ${await login.text()}`).toBeTruthy();
    const senderMe = await jsonOrThrow(
      await sender.request.get(`${API_BASE_URL}/users/me`),
      'sender users/me',
    );
    const senderId = Number(senderMe.ID ?? senderMe.id);

    const anniversary = await createContact(sender.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoAnniversary`,
      lastname: 'Shared',
      anniversary: '2015-06-01',
      emails: [{ type: 'home', value: 'pseudo-anniversary@example.com' }],
    });
    const acceptedGrant = await postJSON(
      sender.request,
      '/share-grants',
      {
        to_user_id: workerId,
        entity_type: 'contact',
        entity_id: anniversary.uid,
        sections: ['emails', 'anniversaries'],
      },
      'anniversary share create',
    );
    const acceptedId = String((acceptedGrant.share_grant as Record<string, unknown>).id);
    await jsonOrThrow(
      await worker.request.post(`${API_BASE_URL}/share-grants/${acceptedId}/accept`),
      'accept anniversary share',
    );

    const pendingContact = await createContact(sender.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoPending`,
      lastname: 'Share',
    });
    await postJSON(
      sender.request,
      '/share-grants',
      {
        to_user_id: workerId,
        entity_type: 'contact',
        entity_id: pendingContact.uid,
        sections: ['emails'],
      },
      'pending share create',
    );

    await postJSON(
      worker.request,
      '/share-grants',
      {
        to_user_id: senderId,
        entity_type: 'contact',
        entity_id: main.uid,
        sections: ['emails'],
      },
      'outgoing share create',
    );

    return {
      seed: {
        contactId: main.ID,
        contactName: mainName,
        activityTitle: 'E2E pseudo coffee',
        occasionLabel: 'E2E pseudo card',
        entityName: 'E2E Pseudo Organisation',
        entityId,
        occasionReminderText: `E2E pseudo card: ${mainName}`,
        acceptedShareLabel: `${anniversary.firstname} ${anniversary.lastname}`,
        pendingShareLabel: `${pendingContact.firstname} ${pendingContact.lastname}`,
        outgoingShareLabel: mainName,
      },
      cleanup: async () => {
        for (const id of [contactId, secondContactId, ...duplicateIds]) {
          if (id) await deleteTestContact(worker.request, id);
        }
        if (entityId) {
          await worker.request.delete(`${API_BASE_URL}/entities/${entityId}`).catch(() => {});
        }
        await deleteThrowawayUser(senderUser.username);
        await worker.close();
        await sender.close();
      },
    };
  } catch (err) {
    await worker.close().catch(() => {});
    await sender.close().catch(() => {});
    if (senderUser.username) await deleteThrowawayUser(senderUser.username).catch(() => {});
    throw err;
  }
}

const test = baseTest.extend<object, { pseudoSeed: PseudoSeed }>({
  pseudoSeed: [
    async ({ browser, workerUser }, provide) => {
      const { seed, cleanup } = await seedWorkerState(browser, workerUser.storageStatePath);
      try {
        await provide(seed);
      } finally {
        await cleanup();
      }
    },
    { scope: 'worker' },
  ],
});

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

interface Scene {
  open: (page: Page, record: RecordScan) => Promise<void>;
  cleanup: () => Promise<void>;
}

interface Surface {
  name: string;
  prepare: (page: Page, seed: PseudoSeed) => Promise<Scene>;
}

const noCleanup = async (): Promise<void> => {};

/**
 * Routes that cannot render a page of their own. Every entry is a pure
 * `<Navigate>` redirect: the target renders (and is scanned separately), while
 * this path immediately leaves. The dedicated test below navigates each one and
 * fails if it ever stops redirecting, so the exemption cannot rot (#2920).
 */
const ROUTE_EXEMPTIONS: Record<string, string> = {
  '/tags': 'Redirect-only compatibility path; /circles?tab=tags is scanned.',
  '/api-tokens':
    'Redirect-only compatibility path; the page moved under /settings, which is scanned.',
  '/register': 'Logged-in shell redirects registration to the dashboard (/), which is scanned.',
};

/**
 * Admin-only pages need the shared admin's session to render their own page
 * rather than redirecting a worker user back to the dashboard. They carry no
 * seeded-fixture dependency, so they run in their own describe below.
 */
const ADMIN_ROUTES: readonly string[] = ['/audit', '/users', '/system-events', '/system-status'];

/** Resolves `:id` route parameters to the seeded fixtures (#2920 item 4). */
function routeFor(route: string, seed: PseudoSeed): string {
  if (route === '/contacts/:id' || route === '/contacts/:id/prep') {
    return route.replace(':id', String(seed.contactId));
  }
  if (route === '/entities/:id') return route.replace(':id', seed.entityId);
  return route;
}

/** Returns the surface for one App.tsx route. */
function routeSurface(route: string): Surface {
  return {
    name: `route ${route}`,
    prepare: async (page, seed) => ({
      open: async (_page, record) => {
        await openPinned(page, routeFor(route, seed));
        // Prove the seeded round-2 state actually rendered: an empty/detail-
        // not-found page would otherwise scan clean and hide a broken seed.
        if (route === '/contacts/:id') {
          await expect(page.getByText(seed.contactName).first()).toBeVisible();
          await expect(page.getByText(seed.occasionLabel).first()).toBeVisible();
        } else if (route === '/contacts/:id/prep') {
          await expect(page.getByText(seed.contactName).first()).toBeVisible();
          await expect(page.getByText(seed.activityTitle).first()).toBeVisible();
        } else if (route === '/activities') {
          await expect(page.getByText(seed.activityTitle).first()).toBeVisible();
        } else if (route === '/shared-with-me') {
          await expect(page.getByText(seed.acceptedShareLabel).first()).toBeVisible();
          await expect(page.getByText(seed.pendingShareLabel).first()).toBeVisible();
        } else if (route === '/shared-by-me') {
          await expect(page.getByText(seed.outgoingShareLabel).first()).toBeVisible();
        } else if (route === '/entities/:id') {
          await expect(page.getByText(seed.entityName).first()).toBeVisible();
        }
        await record('route');
      },
      cleanup: noCleanup,
    }),
  };
}

const DIALOG_SURFACES: readonly Surface[] = [
  {
    name: 'Import contacts (all steps)',
    prepare: async (page, _seed) => {
      const dialogName = pseudo('contacts.import.title');
      const importedName = `${E2E_CONTACT_PREFIX}PseudoImported`;
      return {
        open: async (_page, record) => {
          await openPinned(page, '/contacts');
          await stableClick(page.getByRole('button', { name: pseudo('contacts.import.button') }));
          const dialog = page.getByRole('dialog', { name: dialogName });
          await expect(dialog).toBeVisible();

          await record('upload');
          await dialog.locator('#import-file-input').setInputFiles({
            name: 'pseudo-import.csv',
            mimeType: 'text/csv',
            buffer: Buffer.from(
              `First Name,Last Name,Email\n${importedName},Parsed,pseudo-import@example.com\n`,
            ),
          });
          await expect(
            dialog.getByText(pseudo('contacts.import.mapping.description')),
          ).toBeVisible();
          await record('map columns');

          await dialog.getByRole('button', { name: pseudo('common.continue') }).click();
          // The button is singular for one row and plural for several, so
          // match the shared stem rather than a full `_one`/`_other` template.
          const applyButton = dialog.getByRole('button', {
            name: new RegExp(
              escapeRegExp(pseudo('contacts.import.preview.applyDecisions_one').split(' (')[0]),
            ),
          });
          await expect(applyButton).toBeVisible({ timeout: 15000 });
          await record('review');

          await applyButton.click();
          const doneButton = dialog.getByRole('button', {
            name: pseudo('contacts.import.result.done'),
          });
          await expect(doneButton).toBeVisible({ timeout: 15000 });
          await record('done');
          await doneButton.click();
        },
        cleanup: async () => {
          // The import itself creates a contact; delete exactly the row this
          // run added rather than leaving it for global-setup's next sweep.
          const response = await page.request
            .get(`${API_BASE_URL}/contacts?search=${encodeURIComponent(importedName)}&limit=10`)
            .catch(() => null);
          if (response?.ok()) {
            const body = (await response.json()) as { contacts?: Array<{ id?: number }> };
            for (const contact of body.contacts ?? []) {
              if (contact.id) await deleteTestContact(page.request, contact.id);
            }
          }
        },
      };
    },
  },
  {
    name: 'Account bundle import dialog',
    prepare: async (page, _seed) => ({
      open: async (_page, record) => {
        await openPinned(page, '/settings/data');
        await stableClick(
          page.getByRole('button', { name: pseudo('settings.data.import.mycorrhizalButton') }),
        );
        const dialog = page.getByRole('dialog', {
          name: pseudo('settings.mycorrhizalImport.title'),
        });
        await expect(dialog).toBeVisible();
        await record('connect');
        await dialog.getByRole('button', { name: pseudo('common.cancel') }).click();
        await expect(dialog).toBeHidden();
      },
      cleanup: noCleanup,
    }),
  },
  {
    name: 'Meerkat import (unreachable URL)',
    prepare: async (page, _seed) => ({
      open: async (_page, record) => {
        await openPinned(page, '/settings/data');
        await stableClick(
          page.getByRole('button', { name: pseudo('settings.data.import.meerkatButton') }),
        );
        const dialog = page.getByRole('dialog', { name: pseudo('settings.meerkatImport.title') });
        await expect(dialog).toBeVisible();

        await dialog
          .getByRole('textbox', { name: pseudo('settings.meerkatImport.live.urlLabel') })
          .fill('http://127.0.0.1:9');
        await dialog
          .getByLabel(pseudo('settings.meerkatImport.live.tokenLabel'))
          .fill('unreachable-token');

        const responsePromise = page.waitForResponse(
          (res) =>
            res.request().method() === 'POST' &&
            res.url().includes('/contacts/import/meerkat/connect'),
          { timeout: 15000 },
        );
        await stableClick(
          dialog.getByRole('button', { name: pseudo('settings.meerkatImport.live.connect') }),
        );
        const response = await responsePromise;
        // The server's reason params are prose, not user content. The client
        // interpolates `{{reason}}` into a pseudo-localized template, so the
        // bracket rule alone cannot see the English (#2912).
        const serverProse: string[] = [];
        try {
          const body = (await response.json()) as {
            error?: { reason_params?: Record<string, unknown> };
          };
          for (const value of Object.values(body.error?.reason_params ?? {})) {
            if (typeof value === 'string' && value.trim().split(/\s+/).length >= 4) {
              serverProse.push(value);
            }
          }
        } catch {
          // A non-JSON failure still renders a translated client message; the
          // scan below covers whatever is on screen.
        }
        await expect(dialog.getByRole('alert')).toBeVisible({ timeout: 15000 });
        await record('unreachable', { serverProse });
        await dialog.getByRole('button', { name: pseudo('common.cancel') }).click();
        await expect(dialog).toBeHidden();
      },
      cleanup: noCleanup,
    }),
  },
  {
    name: 'Add contact dialog',
    prepare: async (page, _seed) => ({
      open: async (_page, record) => {
        await openPinned(page, '/contacts');
        await stableClick(
          page.getByRole('button', { name: pseudo('contacts.add.button') }).first(),
        );
        const dialog = page.getByRole('dialog', { name: pseudo('contacts.add.title') });
        await expect(dialog).toBeVisible();
        await record('dialog');
        await dialog.getByRole('button', { name: pseudo('common.cancel') }).click();
        await expect(dialog).toBeHidden();
      },
      cleanup: noCleanup,
    }),
  },
  {
    name: 'Find duplicates dialog',
    prepare: async (page, _seed) => ({
      open: async (_page, record) => {
        await openPinned(page, '/contacts');
        await stableClick(page.getByRole('button', { name: pseudo('contacts.reviewDuplicates') }));
        const dialog = page.getByRole('dialog', { name: pseudo('duplicates.reviewTitle') });
        await expect(dialog).toBeVisible();
        // The seeded same-name/email pair proves the review list itself (not
        // just its empty/loading state) rendered.
        await expect(dialog.getByText('Pair').first()).toBeVisible({ timeout: 15000 });
        await record('dialog');
        await page.keyboard.press('Escape');
        await expect(dialog).toBeHidden();
      },
      cleanup: noCleanup,
    }),
  },
  {
    name: 'Share edit dialog',
    prepare: async (page, seed) => ({
      open: async (_page, record) => {
        await openPinned(page, '/shared-by-me');
        const row = page.getByRole('listitem').filter({ hasText: seed.outgoingShareLabel });
        await expect(row).toBeVisible();
        await row
          .getByRole('button')
          .filter({ hasText: pseudo('standingShares.byMe.edit') })
          .click();
        const dialog = page.getByRole('dialog', {
          name: pseudo('standingShares.byMe.editTitle'),
        });
        await expect(dialog).toBeVisible();
        await record('dialog');
        await dialog.getByRole('button', { name: pseudo('common.cancel') }).click();
        await expect(dialog).toBeHidden();
      },
      cleanup: noCleanup,
    }),
  },
];

// The route table is the router's own, so a new page is scanned the day it
// lands (the same parse check-e2e-routes.mjs uses to require a spec).
const ROUTES = loadRoutes();

// Guard against an import that silently yields an empty list: an empty table
// would make the whole pass vacuously green.
expect(ROUTES.length, 'App.tsx parsed to no routes').toBeGreaterThan(20);

const ROUTE_SURFACES: readonly Surface[] = ROUTES.filter(
  (route) => !Object.keys(ROUTE_EXEMPTIONS).includes(route) && !ADMIN_ROUTES.includes(route),
).map(routeSurface);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Pseudo-localization coverage inventory', () => {
  test('every App.tsx route is either scanned or exempted for a reason', () => {
    for (const route of Object.keys(ROUTE_EXEMPTIONS)) {
      expect(ROUTES, `stale route exemption: ${route} is not a route in App.tsx`).toContain(route);
      expect(ROUTE_EXEMPTIONS[route].length).toBeGreaterThanOrEqual(15);
    }
    const scanned = ROUTE_SURFACES.map((s) => s.name.replace('route ', ''));
    expect(new Set([...scanned, ...ADMIN_ROUTES, ...Object.keys(ROUTE_EXEMPTIONS)])).toEqual(
      new Set(ROUTES),
    );
  });

  test('every route exemption is still a pure redirect', async ({ page }) => {
    for (const route of Object.keys(ROUTE_EXEMPTIONS)) {
      await page.goto(route);
      await page.waitForURL((url) => url.pathname !== route, { timeout: 10000 });
      expect(new URL(page.url()).pathname, `${route} no longer redirects`).not.toBe(route);
    }
  });
});

for (const { label, viewport } of VIEWPORTS) {
  test.describe(`Pseudo-localization (qps, ${label} ${viewport.width}px)`, {
    // This spec owns the text/overflow pass. The automatic per-test axe scan
    // belongs to the populated a11y sweep (#2920 item 6): with the seeded
    // pending share, every route carries the badge that fails #2906's
    // target-size rule, which would bury the translation failures this spec
    // exists to report (and re-scan every route twice).
    annotation: {
      type: SKIP_A11Y_SCAN,
      description: 'pseudo-locale pass; populated a11y sweep is #2920 item 6',
    },
  }, () => {
    test.use({ viewport });

    for (const surface of ROUTE_SURFACES) {
      test(`${surface.name} renders only translated text and does not overflow`, async ({
        page,
        pseudoSeed,
      }) => {
        await enterPseudoLocale(page);
        const scans: ScanRecord[] = [];
        const record: RecordScan = async (step, options) => {
          await expectPseudoLocaleActive(page);
          await settleLayout(page);
          scans.push({
            label: step,
            offences: await scanPage(page, {
              forceScan: [pseudoSeed.occasionReminderText],
              ...options,
            }),
          });
        };
        const scene = await surface.prepare(page, pseudoSeed);
        try {
          await scene.open(page, record);
        } finally {
          await scene.cleanup();
        }
        expectScansClean(
          scans,
          `${surface.name} (${label} ${viewport.width}px)`,
          surface.name === 'route /settings/data' && viewport.width === 600
            ? DATA_SETTINGS_OVERFLOWS
            : [],
        );
      });
    }

    test.describe(`Pseudo-localization admin routes (qps, ${label} ${viewport.width}px)`, {
      annotation: {
        type: SKIP_A11Y_SCAN,
        description: 'pseudo-locale pass; populated a11y sweep is #2920 item 6',
      },
    }, () => {
      // The shared admin renders the real admin pages instead of a
      // non-admin redirect back to the dashboard. These routes need no
      // worker seed, so they do not request `pseudoSeed`.
      test.use({ viewport, sharedUser: true });

      for (const route of ADMIN_ROUTES) {
        test(`route ${route} renders only translated text and does not overflow`, async ({
          page,
        }) => {
          await enterPseudoLocale(page);
          const scans: ScanRecord[] = [];
          const record: RecordScan = async (step, options) => {
            await expectPseudoLocaleActive(page);
            await settleLayout(page);
            scans.push({ label: step, offences: await scanPage(page, options) });
          };
          await openPinned(page, route);
          await record('route');
          expectScansClean(scans, `route ${route} (${label} ${viewport.width}px)`);
        });
      }
    });

    for (const surface of DIALOG_SURFACES) {
      test(`${surface.name} renders only translated text and does not overflow`, async ({
        page,
        pseudoSeed,
      }) => {
        await enterPseudoLocale(page);
        const scans: ScanRecord[] = [];
        const record: RecordScan = async (step, options) => {
          await expectPseudoLocaleActive(page);
          await settleLayout(page);
          scans.push({
            label: step,
            offences: await scanPage(page, {
              forceScan: [pseudoSeed.occasionReminderText],
              ...options,
            }),
          });
        };
        const scene = await surface.prepare(page, pseudoSeed);
        try {
          await scene.open(page, record);
        } finally {
          await scene.cleanup();
        }
        const allowed = [
          ...(surface.name === 'Import contacts (all steps)' ? IMPORT_FIELD_LABEL_OFFENCES : []),
          ...(viewport.width === 600 &&
          (surface.name === 'Account bundle import dialog' ||
            surface.name === 'Meerkat import (unreachable URL)')
            ? DATA_SETTINGS_OVERFLOWS
            : []),
        ];
        expectScansClean(scans, `${surface.name} (${label} ${viewport.width}px)`, allowed);
      });
    }
  });
}
