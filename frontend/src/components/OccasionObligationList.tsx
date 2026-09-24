import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Chip, IconButton, Paper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { OccasionObligation } from '../api/occasionObligations';

interface OccasionObligationListProps {
  obligations: OccasionObligation[];
  onEdit: (obligation: OccasionObligation) => void;
  onDelete: (id: string) => void;
}

// The contact page's occasion-obligation registry (ADR 0024, issue #387):
// every standing card/gift/invite obligation for this contact. Mirrors
// PreferenceList's flat-list shape -- no section grouping needed here, since
// Kind is an open classifier rather than a fixed taxonomy.
export default function OccasionObligationList({
  obligations,
  onEdit,
  onDelete,
}: OccasionObligationListProps) {
  const { t } = useTranslation();

  const handleDeleteClick = (id: string) => {
    if (window.confirm(t('occasions.obligation.deleteMessage'))) {
      onDelete(id);
    }
  };

  if (obligations.length === 0) {
    return (
      <Typography
        variant="body2"
        sx={{
          color: 'text.secondary',
          py: 2,
          textAlign: 'center',
        }}
      >
        {t('occasions.obligation.empty')}
      </Typography>
    );
  }

  const anchorLabel = (o: OccasionObligation): string | null => {
    if (o.anchor_month == null || o.anchor_day == null) return null;
    return `${t(`occasions.months.${o.anchor_month}`)} ${o.anchor_day}`;
  };

  return (
    <Stack spacing={1.5}>
      {obligations.map((o) => (
        <Paper
          key={o.id}
          variant="outlined"
          sx={{
            p: 2,
            bgcolor: o.active ? 'background.paper' : 'action.hover',
          }}
        >
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center', flexWrap: 'wrap' }}>
                <Chip size="small" label={t(`occasions.kinds.${o.kind}`, o.kind)} />
                {anchorLabel(o) && (
                  <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {anchorLabel(o)}
                  </Typography>
                )}
                {o.lead_time_days > 0 && (
                  <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {t('occasions.obligation.leadTimeDisplay', { count: o.lead_time_days })}
                  </Typography>
                )}
                {!o.active && (
                  <Chip
                    size="small"
                    sx={{ height: 18 }}
                    label={t('occasions.obligation.inactive')}
                  />
                )}
                {o.sensitivity !== 'normal' && (
                  <Chip
                    size="small"
                    sx={{ height: 18 }}
                    label={t(`occasions.obligation.sensitivities.${o.sensitivity}`)}
                  />
                )}
              </Box>
              <Typography variant="body1" sx={{ mt: 0.5, overflowWrap: 'anywhere' }}>
                {o.label}
              </Typography>
              {o.notes && (
                <Typography
                  variant="body2"
                  sx={{
                    color: 'text.secondary',
                    mt: 0.5,
                    overflowWrap: 'anywhere',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  {o.notes}
                </Typography>
              )}
            </Box>
            <Box sx={{ display: 'flex', gap: 0.5 }}>
              <IconButton size="small" onClick={() => onEdit(o)} aria-label={t('common.edit')}>
                <EditIcon fontSize="small" />
              </IconButton>
              <IconButton
                size="small"
                color="error"
                onClick={() => handleDeleteClick(o.id)}
                aria-label={t('common.delete')}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Box>
          </Box>
        </Paper>
      ))}
    </Stack>
  );
}
