// Unit tests for the shared accessibility gate's pure rules (issue #2920
// item 5). Each case pins a shape the e2e gate depends on: the extra axe rules
// must block at any impact, and the repeated-name walker must only report a
// real duplicate inside the innermost scope, with narrow exemptions.
import { describe, expect, it } from 'vitest';
import {
  BLOCKING_A11Y_IMPACTS,
  describedByDistinguishes,
  EXTRA_A11Y_RULES,
  findRepeatedAccessibleNames,
  isBlockingA11yViolation,
  isExemptA11yViolationNode,
} from './a11yChecks';

describe('isBlockingA11yViolation', () => {
  it('blocks critical and serious WCAG violations', () => {
    for (const impact of BLOCKING_A11Y_IMPACTS) {
      expect(isBlockingA11yViolation({ id: 'color-contrast', impact })).toBe(true);
    }
  });

  it('does not block moderate/minor violations of ordinary rules', () => {
    expect(isBlockingA11yViolation({ id: 'region', impact: 'moderate' })).toBe(false);
    expect(isBlockingA11yViolation({ id: 'landmark-one-main', impact: 'moderate' })).toBe(false);
    expect(isBlockingA11yViolation({ id: 'x', impact: null })).toBe(false);
  });

  it('blocks the extra rules at any impact, including moderate and null', () => {
    for (const id of EXTRA_A11Y_RULES) {
      expect(isBlockingA11yViolation({ id, impact: 'moderate' })).toBe(true);
      expect(isBlockingA11yViolation({ id, impact: 'minor' })).toBe(true);
      expect(isBlockingA11yViolation({ id, impact: null })).toBe(true);
    }
  });
});

describe('findRepeatedAccessibleNames', () => {
  it('flags two buttons with one name inside a dialog', () => {
    expect(
      findRepeatedAccessibleNames([
        {
          role: 'dialog',
          children: [
            { role: 'button', name: 'Merge' },
            { role: 'button', name: 'Merge' },
            { role: 'button', name: 'Close' },
          ],
        },
      ]),
    ).toEqual([{ scope: 'dialog', name: 'Merge', count: 2 }]);
  });

  it('flags links as well as buttons', () => {
    expect(
      findRepeatedAccessibleNames([
        {
          role: 'main',
          children: [
            { role: 'link', name: 'Open' },
            { role: 'link', name: 'Open' },
          ],
        },
      ]),
    ).toEqual([{ scope: 'main', name: 'Open', count: 2 }]);
  });

  it('does not flag unique names, non-actions, or unnamed controls', () => {
    expect(
      findRepeatedAccessibleNames([
        {
          role: 'main',
          children: [
            { role: 'button', name: 'Save' },
            { role: 'button', name: 'Cancel' },
            { role: 'heading', name: 'Save' },
            { role: 'button' },
          ],
        },
      ]),
    ).toEqual([]);
  });

  it('treats whitespace around a name as the same name', () => {
    expect(
      findRepeatedAccessibleNames([
        {
          role: 'list',
          children: [
            { role: 'button', name: 'Add ' },
            { role: 'button', name: ' Add' },
          ],
        },
      ]),
    ).toEqual([{ scope: 'list', name: 'Add', count: 2 }]);
  });

  it('counts each nested scope separately instead of twice', () => {
    const findings = findRepeatedAccessibleNames([
      {
        role: 'dialog',
        children: [
          { role: 'button', name: 'Close' },
          {
            role: 'list',
            children: [
              { role: 'button', name: 'Edit' },
              { role: 'button', name: 'Edit' },
            ],
          },
        ],
      },
    ]);
    expect(findings).toEqual([{ scope: 'list', name: 'Edit', count: 2 }]);
  });

  it('flags a duplicate in the dialog itself even when a nested list is clean', () => {
    expect(
      findRepeatedAccessibleNames([
        {
          role: 'dialog',
          children: [
            { role: 'button', name: 'Not a duplicate' },
            { role: 'button', name: 'Not a duplicate' },
            {
              role: 'list',
              children: [
                { role: 'button', name: 'Open Alice' },
                { role: 'button', name: 'Open Bob' },
              ],
            },
          ],
        },
      ]),
    ).toEqual([{ scope: 'dialog', name: 'Not a duplicate', count: 2 }]);
  });

  it('allows the row-scoped Remove exemption only inside lists/grids', () => {
    const inList = findRepeatedAccessibleNames([
      {
        role: 'list',
        children: [
          { role: 'button', name: 'Remove' },
          { role: 'button', name: 'Remove' },
        ],
      },
    ]);
    expect(inList).toEqual([]);

    const inDialog = findRepeatedAccessibleNames([
      {
        role: 'dialog',
        children: [
          { role: 'button', name: 'Remove' },
          { role: 'button', name: 'Remove' },
        ],
      },
    ]);
    expect(inDialog).toEqual([{ scope: 'dialog', name: 'Remove', count: 2 }]);
  });

  it('still flags a non-exempt name alongside the exemption', () => {
    expect(
      findRepeatedAccessibleNames([
        {
          role: 'list',
          children: [
            { role: 'button', name: 'Remove' },
            { role: 'button', name: 'Remove' },
            { role: 'button', name: 'Add' },
            { role: 'button', name: 'Add' },
          ],
        },
      ]),
    ).toEqual([{ scope: 'list', name: 'Add', count: 2 }]);
  });

  it('walks text fragments and multiple top-level scopes', () => {
    expect(
      findRepeatedAccessibleNames([
        { role: 'main', children: ['Sign in', { role: 'button', name: 'Go' }] },
        {
          role: 'grid',
          children: [
            { role: 'button', name: 'Delete' },
            { role: 'button', name: 'Delete' },
          ],
        },
      ]),
    ).toEqual([{ scope: 'grid', name: 'Delete', count: 2 }]);
  });
});

describe('isExemptA11yViolationNode', () => {
  it('matches the committed AddContactDialog heading skip only', () => {
    expect(isExemptA11yViolationNode('heading-order', '<h6 class="x">Circles</h6>')).toBe(true);
    expect(
      isExemptA11yViolationNode(
        'heading-order',
        '<h6 class="x">Drag and drop a CSV or VCF file here, or click to select</h6>',
      ),
    ).toBe(false);
    expect(isExemptA11yViolationNode('label-content-name-mismatch', '>Circles</h6>')).toBe(false);
  });
});

describe('describedByDistinguishes', () => {
  it('requires at least two descriptions', () => {
    expect(describedByDistinguishes([])).toBe(false);
    expect(describedByDistinguishes(['alpha'])).toBe(false);
  });

  it('accepts two or more non-empty distinct descriptions', () => {
    expect(describedByDistinguishes(['Alice', 'Bob'])).toBe(true);
    expect(describedByDistinguishes(['Alice', 'Bob', 'Carol'])).toBe(true);
  });

  it('rejects a missing or shared description', () => {
    expect(describedByDistinguishes(['Alice', null])).toBe(false);
    expect(describedByDistinguishes(['Alice', 'Alice'])).toBe(false);
    expect(describedByDistinguishes(['Alice', '  '])).toBe(false);
  });
});
