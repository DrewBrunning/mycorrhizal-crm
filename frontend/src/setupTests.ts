// Extends Vitest's expect with jest-dom matchers (toBeInTheDocument, etc.)
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
    return jsdomFetch(input, init);
  };
}
