// The rule behind assertNoBlockingA11yViolations's single toast-contrast rescan
// (nightly #2086). Kept free of Playwright runtime imports so vitest can unit-test
// it (a11yRetry.vitest.ts): a wrong "retry" silently weakens the app-wide gate.

/** The slice of an axe-core violation this rule reads (structurally compatible). */
export interface AxeViolationLike {
  id: string;
  nodes: { target: ReadonlyArray<unknown>; html: string }[];
}

/** One flagged node as plain data the in-page predicate can take. */
export interface ToastCandidate {
  selector: string | null;
  html: string;
}

/**
 * Maps axe violations to candidates, or null when the rescan is off the table:
 * any violation that is not `color-contrast`, or any multi-part target (iframe /
 * shadow DOM) -- a single `document.querySelector` cannot resolve those.
 */
export function toastRetryCandidates(violations: AxeViolationLike[]): ToastCandidate[] | null {
  const out: ToastCandidate[] = [];
  for (const v of violations) {
    if (v.id !== 'color-contrast') return null;
    for (const node of v.nodes) {
      const [selector] = node.target;
      if (node.target.length !== 1 || typeof selector !== 'string') return null;
      out.push({ selector, html: node.html });
    }
  }
  return out;
}

/**
 * Decides whether every candidate is a toast: a node still in the DOM must sit
 * inside `.MuiSnackbar-root`; one that is gone (the exit transition unmounted it)
 * qualifies only if axe's recorded `html` snippet shows MuiAlert / MuiSnackbar
 * classes. Empty list, bad selector: no.
 *
 * Serialized into `page.evaluate`, so it must stay self-contained -- no closures
 * over module scope, only the global `document`.
 */
export function allCandidatesAreToast(candidates: ToastCandidate[]): boolean {
  if (candidates.length === 0) return false;
  return candidates.every((c) => {
    if (c.selector === null) return false;
    let el: Element | null;
    try {
      el = document.querySelector(c.selector);
    } catch {
      return false;
    }
    if (el === null) return /\bMui(Alert|Snackbar)/.test(c.html);
    return el.closest('.MuiSnackbar-root') !== null;
  });
}
