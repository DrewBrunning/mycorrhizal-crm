// Single source of truth for relationship health band -> color (issue #383,
// ADR-0023). NetworkGraph.tsx, NetworkListView.tsx, and ContactHeader.tsx all
// consume this instead of each reinventing the moss/chanterelle/russula
// mapping.
import type { Theme } from '@mui/material/styles';

// Mirrors backend/models/contact_score.go's ContactScoreResponse.Band values.
export type HealthBand = 'moss' | 'chanterelle' | 'russula';

// Raw theme color for NON-TEXT uses only (canvas fills, list-row dots) --
// see NetworkGraph.tsx's existing circleNodeColor = theme.palette.warning.main
// for the precedent that a raw `.main` fill is fine here. Do NOT use this
// value as literal text color: theme.ts documents that warning.main
// (chanterelle) was only vetted as a fill color, paired with its own
// contrastText, not as freestanding text on an arbitrary background --
// use healthBandChipColor below for any badge/chip TEXT instead.
export function healthBandColor(band: string | undefined, theme: Theme): string | undefined {
  switch (band) {
    case 'moss':
      return theme.palette.success.main;
    case 'chanterelle':
      return theme.palette.warning.main;
    case 'russula':
      return theme.palette.error.main;
    default:
      return undefined;
  }
}

// MUI `color` prop equivalent for badge/chip TEXT -- resolves `main` +
// `contrastText` together, which is what theme.ts's warning/chanterelle
// comment requires (bark text on the light chanterelle fill in both modes).
export type HealthBandChipColor = 'success' | 'warning' | 'error' | undefined;

export function healthBandChipColor(band: string | undefined): HealthBandChipColor {
  switch (band) {
    case 'moss':
      return 'success';
    case 'chanterelle':
      return 'warning';
    case 'russula':
      return 'error';
    default:
      return undefined;
  }
}
