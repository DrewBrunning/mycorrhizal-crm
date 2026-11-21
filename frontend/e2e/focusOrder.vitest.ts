// Unit tests for the keyboard-focus invariants that back the E2E keyboard suite
// (e2e/accessibility.spec.ts). The rules have to stay narrow: a false positive
// here fails a merge-blocking job for a correct screen, and a rule that is too
// permissive silently lets a keyboard trap ship. Each rule is pinned here.
import { describe, expect, it } from 'vitest';
import {
  type FocusStop,
  keyboardFocusViolations,
  positiveTabindexViolations,
  type TabWalk,
} from './focusOrder';

function stop(over: Partial<FocusStop> = {}): FocusStop {
  return {
    label: 'button "Save"',
    inMain: true,
    inDialog: false,
    focusVisible: true,
    rendered: true,
    exposed: true,
    followsPrevious: true,
    ...over,
  };
}

function walk(over: Partial<TabWalk> = {}): TabWalk {
  return {
    stops: [
      stop({ followsPrevious: null }),
      stop({ label: 'link "Contacts"', followsPrevious: true }),
    ],
    completed: true,
    lostFocus: 0,
    stalled: 0,
    steps: 3,
    ...over,
  };
}

describe('keyboardFocusViolations', () => {
  it('accepts a walk that cycles back to its first stop in document order', () => {
    expect(keyboardFocusViolations(walk())).toEqual([]);
  });

  it('fails when Tab never focuses anything', () => {
    const violations = keyboardFocusViolations(walk({ stops: [], completed: false }));
    expect(violations).toHaveLength(1);
    expect(violations[0]).toMatch(/no element received focus/i);
  });

  it('fails when focus never returns to the first stop (a trap)', () => {
    const violations = keyboardFocusViolations(walk({ completed: false }));
    expect(violations.join('\n')).toMatch(/trap|never reached/i);
  });

  it('fails when focus falls to <body> away from the wrap-around boundary', () => {
    const violations = keyboardFocusViolations(walk({ lostFocus: 1 }));
    expect(violations.join('\n')).toMatch(/<body>/);
  });

  it('fails when a Tab press leaves focus on the same element', () => {
    const violations = keyboardFocusViolations(walk({ stalled: 1 }));
    expect(violations.join('\n')).toMatch(/did not move/i);
  });

  it('names an element that received focus with no rendered box', () => {
    const violations = keyboardFocusViolations(
      walk({ stops: [stop({ rendered: false, label: 'button "Ghost"' })] }),
    );
    expect(violations.join('\n')).toContain('button "Ghost"');
    expect(violations.join('\n')).toMatch(/no rendered box|not rendered/i);
  });

  it('names an element that received keyboard focus without :focus-visible', () => {
    const violations = keyboardFocusViolations(
      walk({ stops: [stop({ focusVisible: false, label: 'button "Hidden"' })] }),
    );
    expect(violations.join('\n')).toContain('button "Hidden"');
    expect(violations.join('\n')).toMatch(/focus-visible/i);
  });

  it('fails on a focusable element inside an aria-hidden subtree', () => {
    const violations = keyboardFocusViolations(
      walk({ stops: [stop({ exposed: false, label: 'button "Trap"' })] }),
    );
    expect(violations.join('\n')).toContain('button "Trap"');
    expect(violations.join('\n')).toMatch(/aria-hidden/i);
  });

  it('fails when the tab order inside main runs backwards through the document', () => {
    const violations = keyboardFocusViolations(
      walk({
        stops: [
          stop({ label: 'button "Second"', followsPrevious: null }),
          stop({ label: 'button "First"', followsPrevious: false }),
        ],
      }),
    );
    expect(violations.join('\n')).toContain('button "Second"');
    expect(violations.join('\n')).toContain('button "First"');
    expect(violations.join('\n')).toMatch(/document/i);
  });

  it('ignores document order at a region boundary and inside a dialog', () => {
    // The skip link, app bar and drawer legitimately precede main in the DOM,
    // and a dialog portal is focus-trapped so its internal order may wrap.
    const violations = keyboardFocusViolations(
      walk({
        stops: [
          stop({ label: 'button "In main"', followsPrevious: null }),
          stop({
            label: 'button "In dialog"',
            inMain: false,
            inDialog: true,
            followsPrevious: false,
          }),
          stop({ label: 'button "Later in main"', followsPrevious: true }),
          stop({ label: 'link "Nav"', inMain: false, followsPrevious: false }),
        ],
      }),
    );
    expect(violations).toEqual([]);
  });

  it('does not flag a main-content stop whose predecessor is outside main', () => {
    const violations = keyboardFocusViolations(
      walk({
        stops: [
          stop({ label: 'link "Nav"', inMain: false, followsPrevious: null }),
          stop({ label: 'button "In main"', followsPrevious: false }),
        ],
      }),
    );
    expect(violations).toEqual([]);
  });

  it('treats an unknown document-order verdict as inconclusive, not as a violation', () => {
    // A re-rendered node can lose its marker between stops; the walk reports
    // null rather than guessing.
    const violations = keyboardFocusViolations(
      walk({
        stops: [
          stop({ label: 'button "First"', followsPrevious: null }),
          stop({ label: 'button "Second"', followsPrevious: null }),
        ],
      }),
    );
    expect(violations).toEqual([]);
  });
});

describe('positiveTabindexViolations', () => {
  it('accepts 0 and negative tabindex values', () => {
    expect(
      positiveTabindexViolations([
        { label: 'button "A"', tabIndex: 0 },
        { label: 'main', tabIndex: -1 },
      ]),
    ).toEqual([]);
  });

  it('fails on a positive tabindex and names the element', () => {
    const violations = positiveTabindexViolations([
      { label: 'input#email', tabIndex: 2 },
      { label: 'button "Save"', tabIndex: 0 },
    ]);
    expect(violations).toHaveLength(1);
    expect(violations[0]).toContain('input#email');
    expect(violations[0]).toMatch(/tabindex/i);
  });
});
