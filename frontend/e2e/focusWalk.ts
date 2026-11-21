import type { Page } from '@playwright/test';
import type { FocusStop, TabindexEntry, TabWalk } from './focusOrder';

/**
 * Playwright glue for the keyboard-focus invariants in `focusOrder.ts`.
 * Kept apart from the rules so vitest can import them without loading
 * Playwright (the same split as `a11yRetry.ts`).
 */

/** Marker attributes the walk drops on the elements it visits, cleared at the end. */
const EL_MARKER = 'data-tabwalk-el';
const FIRST_MARKER = 'data-tabwalk-first';
const PREV_MARKER = 'data-tabwalk-prev';

/**
 * The currently focused element, described for the rules. `followsPrevious` is
 * always null here: the walk computes it while it still holds the marker on the
 * previous stop.
 */
export async function describeActiveStop(page: Page): Promise<FocusStop & { tag: string }> {
  return page.evaluate(() => {
    const describe = (node: HTMLElement) => {
      const tag = node.tagName.toLowerCase();
      const id = node.id ? `#${node.id}` : '';
      const role = node.getAttribute('role');
      const text = (node.getAttribute('aria-label') ?? node.textContent ?? '')
        .trim()
        .replace(/\s+/g, ' ');
      const fallback = node.getAttribute('name') ?? node.getAttribute('type') ?? '';
      const name = text || fallback;
      const rect = node.getBoundingClientRect();
      const main = document.getElementById('main-content');
      return {
        label: `${tag}${id}${role ? ` [role=${role}]` : ''}${name ? ` "${name.slice(0, 50)}"` : ''}`,
        inMain: main ? main.contains(node) : false,
        inDialog: node.closest('[role="dialog"]') !== null,
        focusVisible: node.matches(':focus-visible'),
        rendered: rect.width > 0 && rect.height > 0,
        exposed: node.closest('[aria-hidden="true"]') === null && node.closest('[inert]') === null,
      };
    };

    const el = document.activeElement as HTMLElement | null;
    if (!el) {
      return {
        tag: 'none',
        label: 'none',
        inMain: false,
        inDialog: false,
        focusVisible: false,
        rendered: false,
        exposed: false,
        followsPrevious: null,
      };
    }
    return { tag: el.tagName.toLowerCase(), ...describe(el), followsPrevious: null };
  });
}

/**
 * Presses Tab until the page has been covered.
 *
 * Two engine behaviours end the walk: Chromium parks focus on `<body>` between
 * the last and first stop (recorded as the wrap-around boundary, not lost
 * focus), while Firefox leaves `document.activeElement` on the last element and
 * the next Tab goes nowhere. A stalled element is therefore only an end-of-page
 * signal when no tabbable element follows it in the document; otherwise the
 * press genuinely did nothing (a trap or a swallowed Tab). `maxSteps` bounds a
 * page that never resolves either way (the violation, not a hang).
 *
 * Document order is decided per step by comparing the focused element with the
 * marker on the *previous* stop (`compareDocumentPosition`), never by an
 * absolute index: a list that re-renders or a toast that disappears elsewhere
 * on the page shifts every index but leaves relative order intact.
 */
export async function walkTabOrder(page: Page, maxSteps = 120): Promise<TabWalk> {
  const stops: FocusStop[] = [];
  let sawFirst = false;
  let pendingBody = 0;
  let lostFocus = 0;
  let stalled = 0;

  try {
    for (let step = 1; step <= maxSteps; step++) {
      await page.keyboard.press('Tab');
      const active = await page.evaluate(
        ({ elMarker, firstMarker, prevMarker }) => {
          const describe = (node: HTMLElement) => {
            const tag = node.tagName.toLowerCase();
            const id = node.id ? `#${node.id}` : '';
            const role = node.getAttribute('role');
            const text = (node.getAttribute('aria-label') ?? node.textContent ?? '')
              .trim()
              .replace(/\s+/g, ' ');
            const fallback = node.getAttribute('name') ?? node.getAttribute('type') ?? '';
            const name = text || fallback;
            const rect = node.getBoundingClientRect();
            const main = document.getElementById('main-content');
            return {
              label: `${tag}${id}${role ? ` [role=${role}]` : ''}${
                name ? ` "${name.slice(0, 50)}"` : ''
              }`,
              inMain: main ? main.contains(node) : false,
              inDialog: node.closest('[role="dialog"]') !== null,
              focusVisible: node.matches(':focus-visible'),
              rendered: rect.width > 0 && rect.height > 0,
              exposed:
                node.closest('[aria-hidden="true"]') === null && node.closest('[inert]') === null,
            };
          };

          const el = document.activeElement as HTMLElement | null;
          if (!el) {
            return {
              tag: 'none',
              label: 'none',
              inMain: false,
              inDialog: false,
              focusVisible: false,
              rendered: false,
              exposed: false,
              followsPrevious: null,
              sameAsFirst: false,
              stalledNow: false,
            };
          }

          if (!el.hasAttribute(elMarker)) {
            const win = window as unknown as { __tabwalkSeq?: number };
            win.__tabwalkSeq = (win.__tabwalkSeq ?? 0) + 1;
            el.setAttribute(elMarker, String(win.__tabwalkSeq));
          }

          const first = document.querySelector<HTMLElement>(`[${firstMarker}]`);
          const prev = document.querySelector<HTMLElement>(`[${prevMarker}]`);
          const sameAsFirst = first === el;
          const stalledNow = prev === el;
          const followsPrevious =
            prev && prev !== el
              ? (prev.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
              : null;

          if (!first) el.setAttribute(firstMarker, '');
          if (prev && prev !== el) prev.removeAttribute(prevMarker);
          el.setAttribute(prevMarker, '');

          // Firefox leaves focus on the last element instead of wrapping to
          // <body>. Only treat that as end-of-page when nothing tabbable
          // follows it; otherwise the Tab press did nothing.
          const tabbable =
            'a[href], area[href], button, input, select, textarea, iframe, object, embed, ' +
            '[tabindex], audio[controls], video[controls], ' +
            '[contenteditable]:not([contenteditable="false"])';
          const isTabbable = (node: HTMLElement) => {
            if (node.tabIndex < 0 || (node as HTMLButtonElement).disabled) return false;
            if (node.getAttribute('type') === 'hidden') return false;
            if (node.closest('[aria-hidden="true"]') || node.closest('[inert]')) return false;
            const r = node.getBoundingClientRect();
            if (r.width <= 0 || r.height <= 0) return false;
            const style = getComputedStyle(node);
            return style.visibility !== 'hidden' && style.display !== 'none';
          };
          let atEnd = true;
          if (stalledNow) {
            for (const node of document.querySelectorAll<HTMLElement>(tabbable)) {
              if (node === el) continue;
              if ((el.compareDocumentPosition(node) & Node.DOCUMENT_POSITION_FOLLOWING) === 0) {
                continue;
              }
              if (isTabbable(node)) {
                atEnd = false;
                break;
              }
            }
          }

          return {
            tag: el.tagName.toLowerCase(),
            ...describe(el),
            followsPrevious,
            sameAsFirst,
            stalledNow,
            atEnd,
          };
        },
        { elMarker: EL_MARKER, firstMarker: FIRST_MARKER, prevMarker: PREV_MARKER },
      );

      if (active.tag === 'body') {
        pendingBody++;
        continue;
      }

      if (!sawFirst) {
        sawFirst = true;
        pendingBody = 0;
        stops.push(active);
        continue;
      }

      if (active.sameAsFirst) {
        return { stops, completed: true, lostFocus, stalled, steps: step };
      }

      if (pendingBody > 0) {
        lostFocus += pendingBody;
        pendingBody = 0;
      }

      if (active.stalledNow) {
        if (active.atEnd) {
          return { stops, completed: true, lostFocus, stalled, steps: step };
        }
        // A Tab that moved nothing with tabbable elements still ahead is the
        // defect; stop rather than spend the rest of the budget stalling.
        stalled++;
        break;
      }
      stops.push(active);
    }

    return { stops, completed: false, lostFocus, stalled, steps: maxSteps };
  } finally {
    await page.evaluate(
      ({ markers }) => {
        for (const name of markers) {
          for (const node of document.querySelectorAll(`[${name}]`)) {
            node.removeAttribute(name);
          }
        }
        delete (window as unknown as { __tabwalkSeq?: number }).__tabwalkSeq;
      },
      { markers: [EL_MARKER, FIRST_MARKER, PREV_MARKER] },
    );
  }
}

/** Every element carrying an explicit `tabindex` attribute, in DOM order. */
export async function collectTabindexEntries(page: Page): Promise<TabindexEntry[]> {
  return page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLElement>('[tabindex]')).map((el) => {
      const tag = el.tagName.toLowerCase();
      const id = el.id ? `#${el.id}` : '';
      const text = (el.getAttribute('aria-label') ?? el.textContent ?? '')
        .trim()
        .replace(/\s+/g, ' ');
      return {
        label: `${tag}${id}${text ? ` "${text.slice(0, 50)}"` : ''}`,
        tabIndex: el.tabIndex,
      };
    }),
  );
}
