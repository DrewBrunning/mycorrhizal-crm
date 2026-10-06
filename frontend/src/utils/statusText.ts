import type { Theme } from '@mui/material/styles';

// Text-legible status colors for rows that sit on a hover tint.
//
// The light palette's `error.main` (russula, 4.74:1 on the parchment card) and
// `success.main` (moss, 3.86:1 on it) were tuned for fills and large marks. As
// small *text* they fall under WCAG AA (4.5:1) as soon as a `TableRow hover`
// darkens the row background (russula drops to 4.34:1) -- and moss as outlined
// Chip text was already under it at rest. Surfaced by the System Events e2e
// spec's automatic axe scan (issue #1481). The `.dark` variant is the hover
// shade of the same hue and clears AA on both surfaces. Dark mode keeps
// `main`: its status colors are already the bright variants (see theme.ts).
export type StatusTextColor = 'error' | 'success';

export function statusTextColor(color: StatusTextColor) {
  return (theme: Theme): string =>
    theme.palette.mode === 'light' ? theme.palette[color].dark : theme.palette[color].main;
}
