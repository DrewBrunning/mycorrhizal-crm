import { describe, expect, it } from 'vitest';
import {
  DASHBOARD_FOUR_COLUMN_MIN_WIDTH,
  dashboardColumnCount,
  dashboardGridSx,
} from './dashboardGrid';

describe('dashboardColumnCount (#1784)', () => {
  it.each([
    [320, 1],
    [599, 1],
    [600, 2],
    [768, 2],
    [899, 2],
    [900, 2],
    [1199, 2],
    [1200, 2],
    [1399, 2],
    [1400, 4],
    [1536, 4],
    [1920, 4],
  ])('%ipx -> %i columns', (width, cols) => {
    expect(dashboardColumnCount(width)).toBe(cols);
  });

  it('never gives four columns at md (the original overflow)', () => {
    expect(dashboardColumnCount(900)).not.toBe(4);
  });

  it('the CSS breakpoint matches the helper', () => {
    expect(Object.keys(dashboardGridSx)).toContain(
      `@media (min-width:${DASHBOARD_FOUR_COLUMN_MIN_WIDTH}px)`,
    );
  });

  it('uses minmax(0, 1fr) so long words cannot widen the grid', () => {
    const wide = dashboardGridSx[`@media (min-width:${DASHBOARD_FOUR_COLUMN_MIN_WIDTH}px)`];
    for (const v of [
      ...Object.values(dashboardGridSx.gridTemplateColumns),
      wide.gridTemplateColumns,
    ]) {
      expect(v).toContain('minmax(0, 1fr)');
    }
  });
});
