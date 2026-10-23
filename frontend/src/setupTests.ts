// Extends Vitest's expect with jest-dom matchers (toBeInTheDocument, etc.)
import { afterEach } from 'vitest';
import '@testing-library/jest-dom/vitest';

// jsdom does not implement scrolling; App calls window.scrollTo on route changes.
// Guarded because viteConfig.test.ts runs under the node environment (it loads
// Vite's own plugins, which refuse to run under jsdom) and has no window.
if (typeof window !== 'undefined') {
  window.scrollTo = () => {};

  // jsdom's AbortSignal/AbortController are separate classes from Node 22's
  // native globals. apiFetch creates a Node-native AbortController, but
  // jsdom's polyfilled fetch validates `signal instanceof AbortSignal` against
  // its own AbortSignal class — the instanceof check fails and every
  // data-loading component test spews a TypeError. Tests don't need real
  // request timeouts, so strip AbortSignals from fetch calls.
  const jsdomFetch = window.fetch.bind(window);
  window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.signal) {
      const { signal: _, ...rest } = init;
      init = rest;
    }
    // Every real call site fetches relative paths (e.g. `/api/v1/contacts`)
    // and relies on the browser resolving them against the current origin.
    // Vitest's jsdom environment wires `window.fetch` straight to Node's
    // native undici fetch (jsdom itself has never implemented fetch) instead
    // of a browser-style fetch that consults the document's URL, so a
    // relative input throws `TypeError: Failed to parse URL` before the
    // request is even sent — silently short-circuiting every data-loading
    // component test into its catch branch. Resolve against window.location
    // ourselves to restore normal relative-fetch behavior in tests.
    if (typeof input === 'string' && input.startsWith('/')) {
      input = new URL(input, window.location.href).toString();
    }
    return jsdomFetch(input, init);
  };
}

// Framework warnings fail the test that caused them (v1.5.0 review). React and
// MUI report real bugs only through console.error/console.warn -- an
// out-of-range Select value renders a blank control, an unwrapped state
// update can assert against a stale render, invalid DOM nesting is a markup
// bug (frontend trap #3) -- and 137 tests used to print such warnings while
// passing. Application code's own console.error (a caught API error logged
// before showing a message) is NOT a framework warning and is left alone.
//
// A deliberate exception goes in FRAMEWORK_WARNING_ALLOWLIST with a reason.
const FRAMEWORK_WARNING_PATTERNS: RegExp[] = [
  /^Warning: /, // React development warnings (keys, act(), DOM nesting, controlled inputs, …)
  /^MUI: /, // MUI runtime warnings (out-of-range select values, invalid props, …)
  /not wrapped in act\(/,
  /validateDOMNesting/,
];
const FRAMEWORK_WARNING_ALLOWLIST: Array<{ pattern: RegExp; reason: string }> = [];

const frameworkWarnings: string[] = [];
for (const level of ['error', 'warn'] as const) {
  const original = console[level].bind(console);
  console[level] = (...args: unknown[]) => {
    const text = args.map((a) => (typeof a === 'string' ? a : String(a))).join(' ');
    if (
      FRAMEWORK_WARNING_PATTERNS.some((p) => p.test(text)) &&
      !FRAMEWORK_WARNING_ALLOWLIST.some((a) => a.pattern.test(text))
    ) {
      frameworkWarnings.push(`console.${level}: ${text.split('\n')[0]}`);
    }
    original(...args);
  };
}

afterEach(() => {
  if (frameworkWarnings.length === 0) return;
  const found = frameworkWarnings.splice(0);
  throw new Error(
    `Framework warning(s) logged during this test (fix the cause, or allowlist with a reason in setupTests.ts):\n${found.join('\n')}`,
  );
});
