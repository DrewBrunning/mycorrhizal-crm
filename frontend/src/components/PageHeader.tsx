import Box from '@mui/material/Box';
import type { SxProps, Theme } from '@mui/material/styles';
import Typography from '@mui/material/Typography';
import type { ReactNode } from 'react';

/**
 * Issue #1785: the shared "title + actions" header row.
 *
 * Page-header action buttons used to sit in a non-wrapping flex row, so a long
 * uppercase label ("DIAGNOSE AUSFÜHREN", "AUS STANDORTVERLAUF ERFASSEN") pushed
 * the whole page wider than a 360px viewport. This one layout fixes the class:
 *
 * - the row wraps (`flexWrap`, `gap`), so actions drop under the title when the
 *   two do not fit side by side;
 * - the leading block and the actions block are both shrinkable (`minWidth: 0`);
 * - every MUI `Button` inside `actions` may shrink below its content width and
 *   wrap its label onto a second line (`whiteSpace: normal`) instead of forcing
 *   overflow, and reaches the 44px phone touch-target minimum at `xs`.
 *
 * Use it for page titles (`title`) and for in-card header rows (`leading`).
 */
export const PAGE_HEADER_ACTIONS_SX: SxProps<Theme> = {
  display: 'flex',
  flexWrap: 'wrap',
  alignItems: 'center',
  gap: 1,
  minWidth: 0,
  maxWidth: '100%',
  '& .MuiButton-root': {
    minWidth: 0,
    maxWidth: '100%',
    whiteSpace: 'normal',
    textAlign: 'center',
    overflowWrap: 'anywhere',
    // WCAG 2.5.5 / Material: 44px touch target on phones.
    minHeight: { xs: 44, sm: 'auto' },
  },
};

export interface PageHeaderProps {
  /** Rendered as the page's `<h1>`. Ignored when `leading` is given. */
  title?: ReactNode;
  /** Custom leading block (e.g. an `<h2>` plus a status chip) instead of `title`. */
  leading?: ReactNode;
  /** Buttons / controls. Wraps, shrinks and drops under the title on phones. */
  actions?: ReactNode;
  sx?: SxProps<Theme>;
}

export default function PageHeader({ title, leading, actions, sx }: PageHeaderProps) {
  const sxList = Array.isArray(sx) ? sx : sx ? [sx] : [];
  return (
    <Box
      data-testid="page-header"
      sx={[
        {
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          columnGap: 2,
          rowGap: 1,
          mb: 1.5,
          minWidth: 0,
        },
        ...sxList,
      ]}
    >
      {/* flex-basis 16rem (not auto): a long description's max-content width must
          not force the actions onto their own line at desktop widths. */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          flex: '1 1 16rem',
          minWidth: 0,
          maxWidth: '100%',
        }}
      >
        {leading ?? (
          <Typography variant="h5" component="h1" sx={{ overflowWrap: 'anywhere' }}>
            {title}
          </Typography>
        )}
      </Box>
      {actions != null && (
        <Box data-testid="page-header-actions" sx={PAGE_HEADER_ACTIONS_SX}>
          {actions}
        </Box>
      )}
    </Box>
  );
}
