import type { SxProps, Theme } from '@mui/material';

// #1783: MUI's Timeline reserves a fixed share for the date column plus a
// 40px+ dot rail, which left ~90px for the card at phone widths (crushed
// titles, 45px of horizontal page scroll at 320px). Below `sm` the layout is
// single-column instead: the date column is hidden and each card shows its
// own date caption (timelineMobileDateSx), and the rail shrinks to 16px.
// `sm` and up are untouched. Class names are MUI's stable global class names.
export const timelinePhoneSx: SxProps<Theme> = {
  p: { xs: 0, sm: 2 },
  m: 0,
  '& .MuiTimelineItem-root': { minHeight: 'auto' },
  '& .MuiTimelineOppositeContent-root': { display: { xs: 'none', sm: 'block' } },
  '& .MuiTimelineSeparator-root': { width: { xs: 16, sm: 'auto' }, alignItems: 'center' },
  '& .MuiTimelineDot-root': {
    p: { xs: '2px', sm: '4px' },
    mx: { xs: 0, sm: 'auto' },
    '& .MuiSvgIcon-root': { fontSize: { xs: 12, sm: 'inherit' } },
  },
  '& .MuiTimelineContent-root': {
    minWidth: 0,
    px: { xs: 1, sm: 2 },
    pb: { xs: 1.5, sm: 0 },
  },
};

// Small date caption rendered inside each card's TimelineContent; only visible
// below `sm`, where the opposite-content date column is hidden.
export const timelineMobileDateSx: SxProps<Theme> = {
  display: { xs: 'block', sm: 'none' },
  color: 'text.secondary',
  mb: 0.5,
};
