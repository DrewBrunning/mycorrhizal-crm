// Tests that the accessibility claims this palette makes in prose are true of
// the actual theme objects, so a token change that silently drops a pair below
// its claimed ratio fails CI instead of just invalidating the documentation
// (issue #964). The ratios are computed here from the live theme values -- they
// are not copied from assets/colors/README.md, the way a hand-maintained
// expected-value table would be.
//
// Scope of the AAA (7:1) claim: it is about the documented token *pairs*
// (primary/secondary text on the default background, primary text on the card
// surfaces, the brand-primary label), not every string the app renders.
// Secondary text on parchment is deliberately AA-only (6.35:1 light / 6.78:1
// dark) and is asserted at AA. The app-wide rendered-text gate is the AA axe
// scan in e2e/accessibility.spec.ts, plus the AAA-scoped brand-surface scan
// added there; this file is the deterministic half.
import type { Theme } from '@mui/material/styles';
import { describe, expect, test } from 'vitest';
import { darkTheme, lightTheme } from './theme';

// WCAG 2.2 SC 1.4.6 Contrast (Enhanced) is 7:1 for normal text; 1.4.3
// Contrast (Minimum) is 4.5:1; 1.4.11 Non-text Contrast is 3:1 for UI
// components and graphical objects (this is the floor the focus indicator
// must clear -- see #186).
const AAA = 7;
const AA = 4.5;
const NON_TEXT = 3;

const THEMES: ReadonlyArray<readonly [string, Theme]> = [
  ['light', lightTheme],
  ['dark', darkTheme],
];

/** The six semantic fills that carry a `contrastText` label in both themes. */
const SEMANTIC_FILLS = ['primary', 'secondary', 'success', 'warning', 'error', 'info'] as const;

// ---------------------------------------------------------------------------
// WCAG relative-luminance math (the standard formula, not a library).
// ---------------------------------------------------------------------------

function srgbToLinear(channel: number): number {
  const c = channel / 255;
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

function relativeLuminance(hex: string): number {
  const value = hex.replace('#', '');
  const [r, g, b] = [0, 2, 4].map((offset) => parseInt(value.slice(offset, offset + 2), 16));
  return 0.2126 * srgbToLinear(r) + 0.7152 * srgbToLinear(g) + 0.0722 * srgbToLinear(b);
}

function contrastRatio(a: string, b: string): number {
  const [lighter, darker] = [relativeLuminance(a), relativeLuminance(b)].sort((x, y) => y - x);
  return (lighter + 0.05) / (darker + 0.05);
}

/**
 * Reads a component slot's static style override. Throws (rather than
 * silently returning undefined) when the override is absent or a function --
 * a function form cannot be evaluated outside React, and a missing override
 * means the test's assumption about where a color lives is stale.
 */
function staticOverride(theme: Theme, component: string, slot: string): Record<string, unknown> {
  const components = theme.components as unknown as
    | Record<string, { styleOverrides?: Record<string, unknown> }>
    | undefined;
  const override = components?.[component]?.styleOverrides?.[slot];
  if (override === undefined || typeof override === 'function') {
    throw new Error(`${component}.${slot} has no static style override`);
  }
  return override as Record<string, unknown>;
}

function stringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string') {
    throw new Error(`expected a string ${field}, got ${typeof value}`);
  }
  return value;
}

/** The three surface tiers the palette defines: bone, parchment, paper. */
function surfaces(theme: Theme): ReadonlyArray<readonly [string, string]> {
  return [
    ['background.default', theme.palette.background.default],
    ['background.paper', theme.palette.background.paper],
    // The elevated "paper" tone lives in the Dialog override, not in
    // palette.background -- see theme.ts's MuiDialog comment.
    ['elevated paper', stringField(staticOverride(theme, 'MuiDialog', 'paper'), 'backgroundColor')],
  ];
}

/**
 * The effective AppBar foreground/background. Light mode leaves the AppBar on
 * MUI's `color="primary"` default (palette.primary.main + contrastText); dark
 * mode pins a fixed brand-green background with white text (theme.ts's
 * MuiAppBar comment). Read the override when present, fall back otherwise, so
 * this asserts what actually renders rather than a copy of either rule.
 */
function appBarPair(theme: Theme): readonly [string, string] {
  const root = staticOverride(theme, 'MuiAppBar', 'root');
  const background =
    typeof root.backgroundColor === 'string' ? root.backgroundColor : theme.palette.primary.main;
  const foreground =
    typeof root.color === 'string' ? root.color : theme.palette.primary.contrastText;
  return [foreground, background];
}

/**
 * Extracts the focus-outline colour from MuiButtonBase's `&.Mui-focusVisible`
 * override. The value is a hardcoded `Npx solid #RRGGBB` (see #186), not read
 * from the palette, so this parses it out rather than assuming `primary.main`
 * -- if the two ever drift, the assertion below fails.
 */
function focusOutlineColor(theme: Theme): string {
  const root = staticOverride(theme, 'MuiButtonBase', 'root');
  const focusVisible = root['&.Mui-focusVisible'];
  const outline =
    typeof focusVisible === 'object' && focusVisible !== null
      ? (focusVisible as Record<string, unknown>).outline
      : undefined;
  const color = typeof outline === 'string' ? outline.split(/\s+/).pop() : undefined;
  if (!color || !/^#[0-9a-fA-F]{6}$/.test(color)) {
    throw new Error(`could not parse a hex focus-outline colour from ${String(outline)}`);
  }
  return color;
}

function reducedMotionCss(theme: Theme): string {
  const components = theme.components as unknown as
    | Record<string, { styleOverrides?: unknown }>
    | undefined;
  const override = components?.MuiCssBaseline?.styleOverrides;
  if (typeof override !== 'string') {
    throw new Error('MuiCssBaseline.styleOverrides is not a string');
  }
  return override;
}

// ---------------------------------------------------------------------------
// The helper itself is pinned to known WCAG reference values so a broken
// implementation cannot make every assertion below pass vacuously.
// ---------------------------------------------------------------------------

test('the contrast-ratio helper matches the WCAG reference values', () => {
  expect(contrastRatio('#000000', '#FFFFFF')).toBeCloseTo(21, 5);
  expect(contrastRatio('#FFFFFF', '#FFFFFF')).toBeCloseTo(1, 5);
  // #777 on white is the canonical ~4.48:1 "just under AA" example.
  expect(contrastRatio('#777777', '#FFFFFF')).toBeCloseTo(4.48, 2);
  // Ratio is symmetric.
  expect(contrastRatio('#3E543E', '#FFFFFF')).toBeCloseTo(contrastRatio('#FFFFFF', '#3E543E'), 10);
});

// ---------------------------------------------------------------------------
// Documented AAA pairs
// ---------------------------------------------------------------------------

describe.each(THEMES)('%s theme contrast', (_name, theme) => {
  test('primary text clears AAA (7:1) on every surface tier', () => {
    for (const [label, background] of surfaces(theme)) {
      expect(
        contrastRatio(theme.palette.text.primary, background),
        `text.primary on ${label}`,
      ).toBeGreaterThanOrEqual(AAA);
    }
  });

  test('secondary text clears AAA on the default background', () => {
    // README's table claims 7.17:1 (light) / 7.81:1 (dark) here.
    expect(
      contrastRatio(theme.palette.text.secondary, theme.palette.background.default),
    ).toBeGreaterThanOrEqual(AAA);
  });

  test('secondary text clears AA (4.5:1) on the card and elevated surfaces', () => {
    // Not claimed at AAA: soil on parchment is 6.35:1 (light) / 6.78:1 (dark).
    const cardSurfaces = [
      ['background.paper', theme.palette.background.paper],
      [
        'elevated paper',
        stringField(staticOverride(theme, 'MuiDialog', 'paper'), 'backgroundColor'),
      ],
    ] as const;
    for (const [label, background] of cardSurfaces) {
      expect(
        contrastRatio(theme.palette.text.secondary, background),
        `text.secondary on ${label}`,
      ).toBeGreaterThanOrEqual(AA);
    }
  });

  test('the brand-primary label clears AAA (the documented button-label pair)', () => {
    expect(
      contrastRatio(theme.palette.primary.contrastText, theme.palette.primary.main),
    ).toBeGreaterThanOrEqual(AAA);
  });

  test('every semantic fill carries an AA-readable label', () => {
    for (const role of SEMANTIC_FILLS) {
      const { main, contrastText } = theme.palette[role];
      expect(
        contrastRatio(contrastText, main),
        `${role} label on ${role}.main`,
      ).toBeGreaterThanOrEqual(AA);
    }
  });

  test('the focus indicator is the brand primary and clears the 3:1 non-text floor (#186)', () => {
    // The override hardcodes the outline hex instead of referencing the
    // palette, so assert both that it is still the brand primary and that it
    // clears the non-text floor on every surface it is drawn over.
    const outline = focusOutlineColor(theme);
    expect(outline.toLowerCase()).toBe(theme.palette.primary.main.toLowerCase());
    for (const [label, background] of surfaces(theme)) {
      expect(
        contrastRatio(outline, background),
        `focus outline on ${label}`,
      ).toBeGreaterThanOrEqual(NON_TEXT);
    }
  });

  test('the app bar text/background pair clears AAA', () => {
    const [foreground, background] = appBarPair(theme);
    expect(contrastRatio(foreground, background)).toBeGreaterThanOrEqual(AAA);
  });

  test('prefers-reduced-motion is honoured by the global stylesheet (WCAG 2.3.3, AAA)', () => {
    const css = reducedMotionCss(theme);
    // Sanity: parsing the right block, not an unrelated empty string.
    expect(css).toContain('@media (prefers-reduced-motion: reduce)');
    expect(css).toContain('animation-duration: 0.01ms');
    expect(css).toContain('animation-iteration-count: 1');
    expect(css).toContain('transition-duration: 0.01ms');
    expect(css).toContain('scroll-behavior: auto');
  });
});

test('the dark secondary label clears AAA (theme.ts records 7.07:1)', () => {
  expect(
    contrastRatio(darkTheme.palette.secondary.contrastText, darkTheme.palette.secondary.main),
  ).toBeGreaterThanOrEqual(AAA);
});

// The light secondary label is deliberately AA-only (5.54:1) -- bark is the
// best available label for lichen, and white fails at 2.64:1. Pin the floor so
// it cannot silently regress either way.
test('the light secondary label clears AA (theme.ts records 5.54:1)', () => {
  expect(
    contrastRatio(lightTheme.palette.secondary.contrastText, lightTheme.palette.secondary.main),
  ).toBeGreaterThanOrEqual(AA);
});
