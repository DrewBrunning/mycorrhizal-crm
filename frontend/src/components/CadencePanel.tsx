import AddIcon from '@mui/icons-material/Add';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import ScheduleIcon from '@mui/icons-material/Schedule';
import WarningIcon from '@mui/icons-material/Warning';
import { Box, Button, Chip, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { CadencePolicy } from '../api/cadencePolicies';
import { useDateFormat } from '../DateFormatProvider';

interface CadencePanelProps {
  policy: CadencePolicy | null;
  loading: boolean;
  onAdd: () => void;
  onEdit: (policy: CadencePolicy) => void;
  onDelete: (id: string) => void;
}

// The contact-page cadence surface: the derived health read-out ("next due",
// "X days overdue") plus edit/delete, or an empty state that invites setting
// the first cadence. Pure presentation -- all data comes in via props.
export default function CadencePanel({
  policy,
  loading,
  onAdd,
  onEdit,
  onDelete,
}: CadencePanelProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();

  if (loading) {
    return (
      <Paper sx={{ p: 3, textAlign: 'center' }}>
        <Typography variant="body2" color="text.secondary">
          {t('cadence.loading')}
        </Typography>
      </Paper>
    );
  }

  if (!policy) {
    return (
      <Paper sx={{ p: 3, textAlign: 'center' }}>
        <Typography variant="body1" color="text.secondary" sx={{ mb: 2 }}>
          {t('cadence.noPolicy')}
        </Typography>
        <Button
          startIcon={<AddIcon />}
          onClick={onAdd}
          variant="contained"
          color="primary"
          size="small"
        >
          {t('cadence.add')}
        </Button>
      </Paper>
    );
  }

  const health = policy.health;
  const isOverdue = !!health && health.overdue_by > 0;
  const hasInteraction = !!health && health.has_qualifying_interaction;
  const qualifyingTypes = policy.qualifying_types || [];
  const showTypes = qualifyingTypes.length > 0;

  return (
    <Paper sx={{ p: 2 }}>
      <Box display="flex" justifyContent="space-between" alignItems="flex-start">
        <Box>
          <Box display="flex" alignItems="center" gap={1} mb={1}>
            {/* #211: a field label next to the Chip value, not a section heading. */}
            <Typography variant="subtitle2" component="span">
              {t('cadence.interval')}
            </Typography>
            <Chip
              label={t('cadence.intervalValue', { days: policy.target_interval_days })}
              size="small"
            />
          </Box>

          {hasInteraction ? (
            <Box display="flex" alignItems="center" gap={1} mb={0.5}>
              {isOverdue ? (
                <>
                  <WarningIcon fontSize="small" color="warning" />
                  <Typography variant="body2" color="warning.main" fontWeight={500}>
                    {t('cadence.overdueBy', { days: health.overdue_by })}
                  </Typography>
                </>
              ) : (
                <>
                  <CheckCircleIcon fontSize="small" color="success" />
                  <Typography variant="body2" color="success.main">
                    {t('cadence.onTrack')}
                  </Typography>
                </>
              )}
            </Box>
          ) : (
            <Box display="flex" alignItems="center" gap={1} mb={0.5}>
              <ScheduleIcon fontSize="small" color="action" />
              <Typography variant="body2" color="text.secondary">
                {t('cadence.noInteractionsYet')}
              </Typography>
            </Box>
          )}

          {hasInteraction && health?.next_due && (
            <Typography variant="caption" color="text.secondary" display="block">
              {t('cadence.nextDue', { date: formatDate(health.next_due) })}
            </Typography>
          )}
          {hasInteraction && health?.last_interaction && (
            <Typography variant="caption" color="text.secondary" display="block">
              {t('cadence.lastInteraction', { date: formatDate(health.last_interaction) })}
            </Typography>
          )}

          {showTypes && (
            <Box display="flex" flexWrap="wrap" gap={0.5} mt={1}>
              {qualifyingTypes.map((type) => (
                <Chip
                  key={type}
                  label={t(`cadence.types.${type}`, type)}
                  size="small"
                  variant="outlined"
                />
              ))}
            </Box>
          )}
        </Box>

        <Box display="flex" gap={1}>
          <Button startIcon={<EditIcon />} size="small" onClick={() => onEdit(policy)}>
            {t('common.edit')}
          </Button>
          <Button
            startIcon={<DeleteIcon />}
            size="small"
            color="error"
            onClick={() => onDelete(policy.id)}
          >
            {t('common.delete')}
          </Button>
        </Box>
      </Box>
    </Paper>
  );
}
