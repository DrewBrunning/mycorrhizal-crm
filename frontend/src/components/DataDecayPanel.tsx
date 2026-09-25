import AddIcon from '@mui/icons-material/Add';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import PauseCircleOutlineIcon from '@mui/icons-material/PauseCircleOutlined';
import VerifiedIcon from '@mui/icons-material/Verified';
import WarningIcon from '@mui/icons-material/Warning';
import { Box, Button, Chip, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { DataDecayPolicy } from '../api/dataDecayPolicies';
import { useDateFormat } from '../DateFormatProvider';

interface DataDecayPanelProps {
  policy: DataDecayPolicy | null;
  loading: boolean;
  onAdd: () => void;
  onEdit: (policy: DataDecayPolicy) => void;
  onDelete: (id: string) => void;
  onVerify: () => void;
}

// The contact-page data-decay review surface (issue #352): the derived
// health read-out ("next check due", "X days overdue") plus a one-click
// "confirm still current" action, edit/delete, or an empty state that
// invites setting the first policy. Pure presentation -- all data comes in
// via props, mirroring CadencePanel's shape.
export default function DataDecayPanel({
  policy,
  loading,
  onAdd,
  onEdit,
  onDelete,
  onVerify,
}: DataDecayPanelProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();

  if (loading) {
    return (
      <Paper sx={{ p: 3, textAlign: 'center' }}>
        <Typography
          variant="body2"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('dataDecay.loading')}
        </Typography>
      </Paper>
    );
  }

  if (!policy) {
    return (
      <Paper sx={{ p: 3, textAlign: 'center' }}>
        <Typography
          variant="body1"
          sx={{
            color: 'text.secondary',
            mb: 2,
          }}
        >
          {t('dataDecay.noPolicy')}
        </Typography>
        <Button
          startIcon={<AddIcon />}
          onClick={onAdd}
          variant="contained"
          color="primary"
          size="small"
        >
          {t('dataDecay.add')}
        </Button>
      </Paper>
    );
  }

  const health = policy.health;
  const isOverdue = !!health && health.overdue_by > 0;

  return (
    <Paper sx={{ p: 2 }}>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
        }}
      >
        <Box>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              mb: 1,
            }}
          >
            <Typography variant="subtitle2" component="span">
              {t('dataDecay.interval')}
            </Typography>
            <Chip
              label={t('dataDecay.intervalValue', { days: policy.interval_days })}
              size="small"
            />
            {!policy.active && (
              <Chip
                icon={<PauseCircleOutlineIcon fontSize="small" />}
                label={t('dataDecay.paused')}
                size="small"
                variant="outlined"
              />
            )}
          </Box>

          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              mb: 0.5,
            }}
          >
            {isOverdue ? (
              <>
                <WarningIcon fontSize="small" color="warning" />
                <Typography
                  variant="body2"
                  sx={{
                    color: 'warning.main',
                    fontWeight: 500,
                  }}
                >
                  {t('dataDecay.overdueBy', { days: health.overdue_by })}
                </Typography>
              </>
            ) : (
              <>
                <CheckCircleIcon fontSize="small" color="success" />
                <Typography
                  variant="body2"
                  sx={{
                    color: 'success.main',
                  }}
                >
                  {t('dataDecay.onTrack')}
                </Typography>
              </>
            )}
          </Box>

          {health?.next_due && (
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                display: 'block',
              }}
            >
              {t('dataDecay.nextDue', { date: formatDate(health.next_due) })}
            </Typography>
          )}
          <Typography
            variant="caption"
            sx={{
              color: 'text.secondary',
              display: 'block',
            }}
          >
            {policy.last_verified_at
              ? t('dataDecay.lastVerified', { date: formatDate(policy.last_verified_at) })
              : t('dataDecay.neverVerified')}
          </Typography>
        </Box>

        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'flex-end',
            gap: 1,
          }}
        >
          <Button
            startIcon={<VerifiedIcon />}
            size="small"
            variant="contained"
            color="primary"
            onClick={onVerify}
          >
            {t('dataDecay.confirmCurrent')}
          </Button>
          <Box sx={{ display: 'flex', gap: 1 }}>
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
      </Box>
    </Paper>
  );
}
