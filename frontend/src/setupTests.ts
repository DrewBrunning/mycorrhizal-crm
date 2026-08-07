// Extends Vitest's expect with jest-dom matchers (toBeInTheDocument, etc.)
import '@testing-library/jest-dom/vitest';

// jsdom does not implement scrolling; App calls window.scrollTo on route changes.
// Guarded because viteConfig.test.ts runs under the node environment (it loads
// Vite's own plugins, which refuse to run under jsdom) and has no window.
if (typeof window !== 'undefined') {
  window.scrollTo = () => {};
}
