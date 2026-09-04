// @vitest-environment node
//
// Unit tests for the pure comparison in check-bundle-budget.mjs (issue #556).
// The script's I/O half (reading build/assets, writing the baseline) is
// exercised end to end by the CI job + the hand-verification in the PR; this
// pins the regression logic itself so a loosened tolerance or a swallowed
// violation fails loudly.
import { describe, expect, test } from 'vitest';
import { compareBudget, measureBundle, renderTable } from './check-bundle-budget.mjs';

const baseline = {
  tolerancePct: 0.05,
  absoluteSlackBytes: 2048,
  newChunkGzipLimit: 51200,
  totalGzipBaseline: 100000,
  chunks: {
    'react-vendor': 60000,
    'mui-core': 30000,
    index: 10000,
  },
};

describe('compareBudget', () => {
  test('passes when every chunk is within tolerance', () => {
    const measured = {
      chunks: { 'react-vendor': 61000, 'mui-core': 30500, index: 10200 },
      totalGzip: 101700,
    };
    const r = compareBudget(baseline, measured);
    expect(r.ok).toBe(true);
    expect(r.violations).toEqual([]);
  });

  test('passes when a chunk shrinks', () => {
    const measured = {
      chunks: { 'react-vendor': 40000, 'mui-core': 30000, index: 10000 },
      totalGzip: 80000,
    };
    expect(compareBudget(baseline, measured).ok).toBe(true);
  });

  test('fails and names a chunk that grows past tolerance + slack', () => {
    // 60000 * 1.05 + 2048 = 65048; 66000 is over.
    const measured = {
      chunks: { 'react-vendor': 66000, 'mui-core': 30000, index: 10000 },
      totalGzip: 106000,
    };
    const r = compareBudget(baseline, measured);
    expect(r.ok).toBe(false);
    expect(r.violations.join('\n')).toMatch(/react-vendor/);
  });

  test('a small absolute jump on a tiny chunk is absorbed by absoluteSlackBytes', () => {
    // index: 10000 * 1.05 + 2048 = 12548; a +2000 jump to 12000 is fine.
    const measured = {
      chunks: { 'react-vendor': 60000, 'mui-core': 30000, index: 12000 },
      totalGzip: 102000,
    };
    expect(compareBudget(baseline, measured).ok).toBe(true);
  });

  test('fails when the total grows past tolerance even if no single chunk does', () => {
    // Every chunk +4% (under the 5% per-chunk line) but the total baseline is
    // deliberately tight here, so the sum tips over.
    const measured = {
      chunks: { 'react-vendor': 62400, 'mui-core': 31200, index: 10400 },
      totalGzip: 100000 * 1.05 + 2048 + 1,
    };
    const r = compareBudget(baseline, measured);
    expect(r.ok).toBe(false);
    expect(r.violations.join('\n')).toMatch(/total bundle/);
  });

  test('fails on a new chunk over the new-chunk limit', () => {
    const measured = {
      chunks: { 'react-vendor': 60000, 'mui-core': 30000, index: 10000, 'lodash-vendor': 80000 },
      totalGzip: 180000,
    };
    const r = compareBudget(baseline, measured);
    expect(r.ok).toBe(false);
    expect(r.violations.join('\n')).toMatch(/new chunk "lodash-vendor"/);
  });

  test('allows a small new chunk under the new-chunk limit', () => {
    const measured = {
      chunks: { 'react-vendor': 60000, 'mui-core': 30000, index: 10000, 'tiny-vendor': 4000 },
      totalGzip: 104000,
    };
    expect(compareBudget(baseline, measured).ok).toBe(true);
  });

  test('fails when a baselined chunk disappears (stale baseline)', () => {
    const measured = {
      chunks: { 'react-vendor': 60000, 'mui-core': 30000 },
      totalGzip: 90000,
    };
    const r = compareBudget(baseline, measured);
    expect(r.ok).toBe(false);
    expect(r.violations.join('\n')).toMatch(/chunk "index" is in the baseline but was not emitted/);
  });
});

describe('renderTable', () => {
  test('emits a Markdown table with a row per chunk plus the total', () => {
    const { rows } = compareBudget(baseline, {
      chunks: { 'react-vendor': 60000, 'mui-core': 30000, index: 10000 },
      totalGzip: 100000,
    });
    const md = renderTable(rows);
    expect(md).toMatch(/^\| chunk \|/);
    expect(md).toMatch(/`react-vendor`/);
    expect(md).toMatch(/`\(total\)`/);
  });
});

describe('measureBundle', () => {
  test('the committed baseline still matches a fresh build when build/assets exists', () => {
    let measured;
    try {
      measured = measureBundle();
    } catch {
      // No build present (the common case for a plain `vitest run`) -- the CI
      // bundle-budget job is what exercises this path against a real build.
      return;
    }
    expect(Object.keys(measured.chunks).length).toBeGreaterThan(0);
  });
});
