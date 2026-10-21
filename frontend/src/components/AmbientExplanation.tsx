import { Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { AmbientReason } from '../api/contactScore';

interface AmbientExplanationProps {
  reason?: AmbientReason;
  // 'body2' for the primary line in a panel, 'caption' for a secondary note.
  variant?: 'body2' | 'caption';
}

// Issue #1735 (backend #1733): the one-line explanation shown wherever an
// ambient contact would otherwise display days-since-last-interaction or an
// overdue badge. `reason` is 'household' (derived from a current household
// membership) or 'flag' (the explicit crm.ambient toggle); an absent/unknown
// reason falls back to the flag wording.
export default function AmbientExplanation({ reason, variant = 'body2' }: AmbientExplanationProps) {
  const { t } = useTranslation();
  return (
    <Typography
      variant={variant}
      sx={{ color: 'text.secondary' }}
      data-testid="ambient-explanation"
    >
      {reason === 'household' ? t('ambient.reasonHousehold') : t('ambient.reasonFlag')}
    </Typography>
  );
}
