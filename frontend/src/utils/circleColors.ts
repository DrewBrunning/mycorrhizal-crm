import type { CircleColor } from '../api/circles';

// Palette token -> display colour (issue #1995). Mid-tone values that read on
// both light and dark surfaces; the backend stores only the token, so web and
// Android may render their own shade of each.
export const CIRCLE_COLOR_HEX: Record<Exclude<CircleColor, ''>, string> = {
  red: '#e53935',
  orange: '#fb8c00',
  yellow: '#fdd835',
  green: '#43a047',
  teal: '#00897b',
  blue: '#1e88e5',
  purple: '#8e24aa',
  pink: '#d81b60',
  grey: '#757575',
};

export function circleColorHex(color: CircleColor | undefined): string | undefined {
  return color ? CIRCLE_COLOR_HEX[color] : undefined;
}
