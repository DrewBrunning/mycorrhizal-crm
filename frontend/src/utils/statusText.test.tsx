import { Chip, ThemeProvider } from '@mui/material';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';
import { darkTheme, lightTheme } from '../theme';
import { statusTextColor } from './statusText';

afterEach(cleanup);

// Contrast of two #rrggbb colors (WCAG 2 relative luminance).
function luminance(hex: string): number {
  const ch = [1, 3, 5].map((i) => {
    const c = Number.parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * ch[0] + 0.7152 * ch[1] + 0.0722 * ch[2];
}
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe('statusTextColor', () => {
  test('light mode uses the .dark shade, dark mode keeps main', () => {
    expect(statusTextColor('error')(lightTheme)).toBe(lightTheme.palette.error.dark);
    expect(statusTextColor('success')(lightTheme)).toBe(lightTheme.palette.success.dark);
    expect(statusTextColor('error')(darkTheme)).toBe(darkTheme.palette.error.main);
    expect(statusTextColor('success')(darkTheme)).toBe(darkTheme.palette.success.main);
  });

  // The reason the helper exists: the light `.main` colors miss AA as small
  // text on the parchment card (success) / its hover tint (error); `.dark`
  // clears it. Surfaces are the measured ones from the axe reports.
  test('the light shades clear WCAG AA (4.5:1) where .main did not', () => {
    expect(contrast(lightTheme.palette.success.main, '#efe7d9')).toBeLessThan(4.5);
    expect(contrast(lightTheme.palette.error.main, '#e5ded0')).toBeLessThan(4.5);
    expect(contrast(lightTheme.palette.success.dark, '#efe7d9')).toBeGreaterThanOrEqual(4.5);
    expect(contrast(lightTheme.palette.error.dark, '#e5ded0')).toBeGreaterThanOrEqual(4.5);
  });

  test('applies as a Chip text color through sx', () => {
    render(
      <ThemeProvider theme={lightTheme}>
        <Chip
          label="OK"
          color="success"
          variant="outlined"
          sx={{ color: statusTextColor('success') }}
        />
      </ThemeProvider>,
    );
    const label = screen.getByText('OK');
    expect(getComputedStyle(label.parentElement as HTMLElement).color).toBe('rgb(11, 118, 67)');
  });
});
