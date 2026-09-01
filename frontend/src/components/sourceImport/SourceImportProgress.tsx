import { Box, LinearProgress, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { SourceImportStatus } from '../../api/sourceImport';

interface Props {
  status: SourceImportStatus | null;
  // Human name of the source, e.g. "Monica" — for the copy.
  sourceLabel: string;
}

// The fetch/import progress step: a phased bar plus a note that the import
// keeps running server-side, so closing the dialog and coming back is safe.
export default function SourceImportProgress({ status, sourceLabel }: Props) {
  const { t } = useTranslation();

  const phase = status?.phase ?? 'connecting';
  const done = status?.phase_done ?? 0;
  const total = status?.phase_total ?? 0;
  const determinate = total > 0;
  const pct = determinate ? Math.min(100, Math.round((done / total) * 100)) : 0;

  return (
    <Box sx={{ py: 2 }}>
      <Typography variant="body1" gutterBottom>
        {t(`settings.sourceImport.phase.${phase}`, { source: sourceLabel })}
      </Typography>
      <LinearProgress
        variant={determinate ? 'determinate' : 'indeterminate'}
        value={pct}
        sx={{ my: 1, height: 8, borderRadius: 1 }}
      />
      {determinate && (
        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
          {t('settings.sourceImport.progress.count', { done, total })}
        </Typography>
      )}
      <Typography variant="body2" sx={{ color: 'text.secondary', mt: 2 }}>
        {t('settings.sourceImport.progress.keepsRunning')}
      </Typography>
    </Box>
  );
}
