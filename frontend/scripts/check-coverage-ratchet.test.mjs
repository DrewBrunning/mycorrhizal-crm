// @vitest-environment node
//
// Unit tests for the pure comparison in check-coverage-ratchet.mjs (the
// per-file coverage no-regression ratchet). The script's I/O half (reading
// coverage/coverage-summary.json, writing the baseline) is exercised end to
// end by the CI job + hand-verification in the PR; this pins the regression
// logic itself so a loosened tolerance or a swallowed drop fails loudly.
import { describe, expect, test } from 'vitest';
import { compareCoverage, renderTable } from './check-coverage-ratchet.mjs';

const baseline = {
  tolerancePct: 1.5,
  files: {
    'src/hooks/useContacts.ts': { lines: 97.9, branches: 88 },
    'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
    'src/AuditPage.tsx': { lines: 60, branches: 40 },
  },
};

describe('compareCoverage', () => {
  test('passes when every file holds or improves', () => {
    const current = {
      'src/hooks/useContacts.ts': { lines: 98, branches: 90 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      'src/AuditPage.tsx': { lines: 60, branches: 40 },
    };
    const r = compareCoverage(baseline, current);
    expect(r.ok).toBe(true);
    expect(r.violations).toEqual([]);
  });

  test('passes on a drop within tolerance', () => {
    // 97.9 -> 96.5 is a 1.4pt drop, under the 1.5pt tolerance.
    const current = {
      'src/hooks/useContacts.ts': { lines: 96.5, branches: 88 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      'src/AuditPage.tsx': { lines: 60, branches: 40 },
    };
    expect(compareCoverage(baseline, current).ok).toBe(true);
  });

  test('fails and names a file whose line coverage drops past tolerance', () => {
    const current = {
      'src/hooks/useContacts.ts': { lines: 97.9, branches: 88 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      // 60 -> 50 is a 10pt drop -- e.g. a deleted test.
      'src/AuditPage.tsx': { lines: 50, branches: 40 },
    };
    const r = compareCoverage(baseline, current);
    expect(r.ok).toBe(false);
    expect(r.violations.join('\n')).toMatch(/AuditPage\.tsx: lines coverage dropped/);
  });

  test('fails and names a file whose branch coverage drops past tolerance even if lines held', () => {
    const current = {
      'src/hooks/useContacts.ts': { lines: 97.9, branches: 88 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      // lines unchanged, branches 40 -> 20: the removed test only dropped an
      // else-arm's coverage, not the line count.
      'src/AuditPage.tsx': { lines: 60, branches: 20 },
    };
    const r = compareCoverage(baseline, current);
    expect(r.ok).toBe(false);
    expect(r.violations.join('\n')).toMatch(/AuditPage\.tsx: branches coverage dropped/);
  });

  test('a new file with no baseline entry is not gated', () => {
    const current = {
      'src/hooks/useContacts.ts': { lines: 97.9, branches: 88 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      'src/AuditPage.tsx': { lines: 60, branches: 40 },
      'src/NewFeature.tsx': { lines: 5, branches: 0 },
    };
    const r = compareCoverage(baseline, current);
    expect(r.ok).toBe(true);
    const row = r.rows.find((row) => row.name === 'src/NewFeature.tsx');
    expect(row.status).toBe('new');
  });

  test('a removed/renamed file drops out of the baseline without failing', () => {
    const current = {
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      'src/AuditPage.tsx': { lines: 60, branches: 40 },
    };
    const r = compareCoverage(baseline, current);
    expect(r.ok).toBe(true);
    const row = r.rows.find((row) => row.name === 'src/hooks/useContacts.ts');
    expect(row.status).toBe('gone');
  });

  test('improving coverage never fails regardless of magnitude', () => {
    const current = {
      'src/hooks/useContacts.ts': { lines: 100, branches: 100 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      'src/AuditPage.tsx': { lines: 100, branches: 100 },
    };
    expect(compareCoverage(baseline, current).ok).toBe(true);
  });
});

describe('renderTable', () => {
  test('lists only the changed/new/gone rows, not every ok file', () => {
    const current = {
      'src/hooks/useContacts.ts': { lines: 97.9, branches: 88 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      'src/AuditPage.tsx': { lines: 50, branches: 40 },
    };
    const { rows } = compareCoverage(baseline, current);
    const md = renderTable(rows);
    expect(md).toMatch(/AuditPage\.tsx/);
    expect(md).not.toMatch(/useContacts\.ts/);
  });

  test('reports a clean message when nothing changed', () => {
    const { rows } = compareCoverage(baseline, {
      'src/hooks/useContacts.ts': { lines: 97.9, branches: 88 },
      'src/utils/clipboard.ts': { lines: 94.4, branches: 100 },
      'src/AuditPage.tsx': { lines: 60, branches: 40 },
    });
    expect(renderTable(rows)).toMatch(/held or improved/);
  });
});
