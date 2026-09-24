// Single source of truth for relationship health band -> color (issue #383,
// ADR-0023). NetworkGraph.tsx, NetworkListView.tsx, and ContactHeader.tsx all
// consume this instead of each reinventing the moss/chanterelle/russula
// mapping.
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import type { SvgIconProps } from '@mui/material';
import type { Theme } from '@mui/material/styles';
import type { ComponentType } from 'react';

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

// A distinct icon SHAPE per band, not just a color -- matches
// CadencePanel.tsx's existing convention (CheckCircleIcon/WarningIcon for
// its own on-track/overdue readout) for exactly the same reason NetworkListView
// pairs its dot with a text label: color alone can't carry status for a
// colorblind sighted user, but a differently-shaped icon can, independent of
// hue. Used for the compact space-constrained presentation (ContactHeader's
// narrow layout) in place of the full-text Chip the wide layout has room for.
export function healthBandIcon(band: string | undefined): ComponentType<SvgIconProps> | undefined {
  switch (band) {
    case 'moss':
      return CheckCircleIcon;
    case 'chanterelle':
      return WarningIcon;
    case 'russula':
      return ErrorIcon;
    default:
      return undefined;
  }
}

// Issue #1193: the color for a deceased contact's node/dot, wherever a
// health-band color would otherwise apply. Deliberately NOT one of the
// moss/chanterelle/russula health colors (a deceased contact isn't a health
// verdict) and NOT a bright accent like laccaria/info either -- a deceased
// contact is inactive/historical, not a distinct status to draw the eye to.
// text.secondary ("soil") is this codebase's existing muted-but-legible
// token (already used for NetworkListView's band-label captions), so this
// reads as "present but quiet" rather than a fourth status color.
export function deceasedNodeColor(theme: Theme): string {
  return theme.palette.text.secondary;
}
