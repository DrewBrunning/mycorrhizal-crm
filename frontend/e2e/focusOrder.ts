/**
 * Keyboard-focus invariants (the automated half of WCAG 2.1.1 Keyboard, 2.4.3
 * Focus Order and 2.4.7 Focus Visible).
 *
 * axe-core cannot see any of this: it reads the static DOM, so a focus trap, a
 * lost focus point or a positive `tabindex` all pass its scan. The Playwright
 * glue that produces a [TabWalk] lives in `focusWalk.ts`; the rules live here
 * so they can be unit-tested in jsdom (`focusOrder.vitest.ts`) without a
 * browser, the same split as `a11yRetry.ts`.
 */
import type { Page } from '@playwright/test';

/** One element that received focus from a `Tab` press. */
export interface FocusStop {
  /** Human label for failure messages, e.g. `button "Add contact"`. */
  label: string;
  /** Whether the stop is inside the `#main-content` landmark. */
  inMain: boolean;
  /** Whether the stop is inside an open `[role="dialog"]`. */
  inDialog: boolean;
  /** Whether the element matched `:focus-visible` after the Tab press. */
  focusVisible: boolean;
  /** Whether the element has a non-zero rendered box. */
  rendered: boolean;
  /** Whether the element is reachable by assistive tech (not aria-hidden/inert). */
  exposed: boolean;
  /**
   * Whether the browser places this stop after the previous one in document
   * order. `null` for the first stop, and when the marker on the previous
   * element was lost to a re-render (inconclusive, not a violation).
   */
  followsPrevious: boolean | null;
}

/**
 * One pass through a page's tab order.
 *
 * `stops` excludes `<body>`: a browser hands focus to the document body between
 * the last and first stop of a page (Chromium under Playwright does), which is
 * the wrap-around boundary and not a bug. A body hand-off anywhere else is a
 * lost focus point, counted in `lostFocus`.
 */
export interface TabWalk {
  stops: FocusStop[];
  /**
   * Whether the walk reached the end of the page: either focus wrapped back to
   * the first stop (Chromium) or the stalled element had no tabbable successor
   * (Firefox parks focus instead of wrapping).
   */
  completed: boolean;
  /** Body hand-offs away from the wrap-around boundary. */
  lostFocus: number;
  /** Tab presses that left focus on the element it was already on. */
  stalled: number;
  /** Number of Tab presses taken. */
  steps: number;
}

/**
 * Every keyboard-violation message for a walk, or an empty array.
 *
 * Deliberately narrow so a correct page never fails:
 * - the wrap-around only counts as closed when focus returns to the *first*
 *   stop after at least one stop was recorded;
 * - the document-order rule compares consecutive stops only when both are
 *   inside `#main-content` and neither is in a dialog. The app bar, skip link
 *   and drawer legitimately precede `main` in the DOM, and a dialog portal is
 *   focus-trapped, so its internal order may wrap;
 * - `:focus-visible` is checked rather than a computed outline: indicator
 *   heuristics (outline/box-shadow/border) false-positive on MUI's ripple and
 *   on controls that draw the indicator on an ancestor.
 */
export function keyboardFocusViolations(walk: TabWalk): string[] {
  const violations: string[] = [];

  if (walk.stops.length === 0) {
    violations.push('no element received focus from Tab (nothing is keyboard-reachable)');
    return violations;
  }

  if (!walk.completed) {
    violations.push(
      `Tab did not reach the end of the page after ${walk.steps} presses — a keyboard trap ` +
        'or an unreachable tail',
    );
  }

  if (walk.lostFocus > 0) {
    violations.push(
      `focus fell to <body> ${walk.lostFocus} time(s) away from the wrap-around boundary ` +
        '(focus was lost between stops)',
    );
  }

  if (walk.stalled > 0) {
    violations.push(`focus did not move on ${walk.stalled} Tab press(es)`);
  }

  for (const stop of walk.stops) {
    if (!stop.exposed) {
      violations.push(`${stop.label} received focus but is inside an aria-hidden/inert subtree`);
    }
    if (!stop.rendered) {
      violations.push(`${stop.label} received focus with no rendered box (not rendered)`);
    }
    if (!stop.focusVisible) {
      violations.push(`${stop.label} received keyboard focus without :focus-visible`);
    }
  }

  for (let i = 1; i < walk.stops.length; i++) {
    const previous = walk.stops[i - 1];
    const current = walk.stops[i];
    if (!previous.inMain || !current.inMain || previous.inDialog || current.inDialog) continue;
    if (current.followsPrevious === false) {
      violations.push(
        `${current.label} follows ${previous.label} in the tab order but precedes it in the ` +
          'document (positive tabindex or a reordered portal)',
      );
    }
  }

  return violations;
}

/**
 * The focused element, reduced to the facts the row-removal rule needs.
 *
 * `label` is for failure messages only. It is read from attributes and text, so
 * a detached node (the opener after its list row unmounts) can still describe
 * itself without touching layout or throwing.
 */
export interface ActiveElementState {
  tag: string;
  label: string;
  /** The document body or root element: no control holds focus. */
  isBody: boolean;
  /** The element is still in the document (a removed row's button is not). */
  isConnected: boolean;
}

/**
 * Why focus is lost after an action removed the control that had it, or null.
 *
 * A browser moves focus to `<body>` when the focused element is removed from
 * the document (Chromium does it immediately). Some engines/edge cases leave
 * `document.activeElement` pointing at the detached node instead, which is
 * just as lost to a keyboard user, so both are violations. The rule is
 * deliberately narrow: it does not require a particular destination -- that is
 * each action's own acceptance criterion (#2918) -- only that focus landed
 * somewhere a keyboard user can carry on from.
 */
export function focusLossViolation(active: ActiveElementState | null): string | null {
  if (!active || active.tag === 'none') {
    return 'focus is lost: document.activeElement is null (nothing holds focus)';
  }
  if (active.isBody || active.tag === 'body' || active.tag === 'html') {
    return 'focus fell to <body> after the action removed the focused control';
  }
  if (!active.isConnected) {
    return `focus stayed on a detached ${active.label} that the action removed from the document`;
  }
  return null;
}

/**
 * One destructive action that removes or replaces its own row, and the spec
 * that proves focus survives it.
 *
 * This is a reviewed inventory, not a derived list: a new action that removes
 * the control the user just pressed must add an entry here and an
 * `expectFocusNotLost(page, '<action>')` call to its spec. `focusOrder.vitest.ts`
 * fails on a call that is missing here, and on an entry no spec makes, so the
 * list cannot silently go stale in either direction.
 */
export interface RowRemovingAction {
  /** Exact tag passed to `expectFocusNotLost`; unique across the suite. */
  action: string;
  /** Spec file (under `e2e/`) that performs the action and asserts focus. */
  spec: string;
  /** What disappears, for the next person adding a similar action. */
  removes: string;
}

export const ROW_REMOVING_ACTIONS: readonly RowRemovingAction[] = [
  {
    action: 'share Remove',
    spec: 'contactShares.spec.ts',
    removes: 'a removed (ended) grant row in Shared by me',
  },
  {
    action: 'share re-confirm',
    spec: 'contactShares.spec.ts',
    removes: 'the row’s “Review and re-confirm” button when the grant is confirmed',
  },
  {
    action: 'contacts: Not a duplicate',
    spec: 'duplicateScan.spec.ts',
    removes: 'a pair row in the Review duplicates dialog',
  },
  {
    action: 'organisations: Not a duplicate',
    spec: 'entities.spec.ts',
    removes: 'a pair row in the Possible duplicates panel',
  },
  {
    action: 'Trash: Restore',
    spec: 'trash.spec.ts',
    removes: 'a restored contact row from the Trash list',
  },
  {
    action: 'Trash: Delete forever',
    spec: 'trash.spec.ts',
    removes: 'a purged contact row from the Trash list',
  },
  {
    action: 'reminder delete',
    spec: 'reminders.spec.ts',
    removes: 'a deleted reminder card from a contact page',
  },
];

/**
 * The active element's tag/label and whether it is body/root, detached, or
 * still a usable focus target.
 */
export async function readActiveElement(page: Page): Promise<ActiveElementState> {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el) return { tag: 'none', label: 'none', isBody: true, isConnected: false };
    const tag = el.tagName ? el.tagName.toLowerCase() : 'unknown';
    const text = (el.getAttribute('aria-label') ?? el.textContent ?? '')
      .trim()
      .replace(/\s+/g, ' ');
    return {
      tag,
      label: `${tag}${el.id ? `#${el.id}` : ''}${text ? ` "${text.slice(0, 50)}"` : ''}`,
      isBody: tag === 'body' || tag === 'html',
      isConnected: el.isConnected,
    };
  });
}

/**
 * Asserts the action that just removed its own row left focus on a real,
 * connected control rather than dropping it to `<body>` (issue #2918).
 *
 * Call it after the row is gone (and any dialog is closed), not immediately
 * after the click: the action's state update is what unmounts the opener, and
 * before that lands, focus is legitimately still on the button.
 *
 * Throws rather than using Playwright's `expect` so this module stays
 * importable by the jsdom unit tests (`focusOrder.vitest.ts`); the message
 * still lands in the Playwright failure output.
 */
export async function expectFocusNotLost(page: Page, action: string): Promise<void> {
  const violation = focusLossViolation(await readActiveElement(page));
  if (violation) throw new Error(`${action}: ${violation}`);
}

/** An element carrying an explicit `tabindex` attribute. */
export interface TabindexEntry {
  label: string;
  tabIndex: number;
}

/** A positive `tabindex` overrides document order and breaks 2.4.3. */
export function positiveTabindexViolations(entries: TabindexEntry[]): string[] {
  return entries
    .filter((entry) => entry.tabIndex > 0)
    .map(
      (entry) =>
        `${entry.label} sets tabindex="${entry.tabIndex}" — a positive tabindex overrides the ` +
        'document tab order',
    );
}
