// Pins which repeated-accessible-name checks the populated sweep runs per
// route (#2920 review): the shell chrome on every route, and the whole page on
// the populated contact detail, so a #2911-class duplicate outside the three
// states accessibleNames.spec.ts scans is caught by the nightly sweep.
import { describe, expect, it } from 'vitest';
import { REPEATED_NAME_CHROME_SELECTOR, repeatedNameContextsFor } from './populatedRoutes';

describe('repeatedNameContextsFor', () => {
  it('always checks the app bar and navigation chrome', () => {
    expect(REPEATED_NAME_CHROME_SELECTOR).toContain('header');
    expect(REPEATED_NAME_CHROME_SELECTOR).toContain('nav');
    for (const route of ['/', '/contacts', '/settings', '/contacts/:id']) {
      expect(repeatedNameContextsFor(route)).toContain(REPEATED_NAME_CHROME_SELECTOR);
    }
  });

  it('checks the whole page on the populated contact detail only', () => {
    expect(repeatedNameContextsFor('/contacts/:id')).toContain('body');
    expect(repeatedNameContextsFor('/contacts')).not.toContain('body');
  });
});
