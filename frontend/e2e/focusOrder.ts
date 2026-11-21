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
