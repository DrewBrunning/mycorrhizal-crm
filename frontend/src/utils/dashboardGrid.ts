// Issue #1784: the dashboard section grid. The permanent sidebar (256px, from
// `md`/900px up) eats a quarter of the viewport, so four columns at `md` left
// ~150px per column and overflowed the page in German. Four columns need a
// viewport of roughly 1400px (>= ~265px per column once the sidebar and page
// padding are subtracted); below that two columns, and one column on phones.
export const DASHBOARD_FOUR_COLUMN_MIN_WIDTH = 1400;
export const DASHBOARD_TWO_COLUMN_MIN_WIDTH = 600;

// `minmax(0, 1fr)` rather than `1fr`: a bare `1fr` has an automatic minimum of
// the item's min-content width, so one long unbreakable word forces the grid
// wider than the page.
// Spread into an `sx` prop. The wide-screen rule is a top-level media-query key
// (a raw `@media` key is not valid inside a responsive-value object).
export const dashboardGridSx = {
  display: 'grid',
  gridTemplateColumns: {
    xs: 'minmax(0, 1fr)',
    sm: 'repeat(2, minmax(0, 1fr))',
  },
  [`@media (min-width:${DASHBOARD_FOUR_COLUMN_MIN_WIDTH}px)`]: {
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
  },
} as const;

/** Column count the grid renders at a given viewport width (px). */
export function dashboardColumnCount(viewportWidth: number): 1 | 2 | 4 {
  if (viewportWidth >= DASHBOARD_FOUR_COLUMN_MIN_WIDTH) return 4;
  if (viewportWidth >= DASHBOARD_TWO_COLUMN_MIN_WIDTH) return 2;
  return 1;
}
