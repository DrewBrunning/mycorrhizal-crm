import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import GroupIcon from '@mui/icons-material/Group';
import { Box, Button, Chip, IconButton, Paper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { OccasionEvent } from '../api/occasionEvents';

interface OccasionEventListProps {
  events: OccasionEvent[];
  onEdit: (event: OccasionEvent) => void;
  onDelete: (id: string) => void;
  onManageAttendees: (event: OccasionEvent) => void;
}

// formatEventWhen renders the start (and optional end) as a locale string. The
// event is a moment in time (RFC 3339), so the browser's zone is correct here.
export function formatEventWhen(event: OccasionEvent): string {
  const start = new Date(event.starts_at);
  if (Number.isNaN(start.getTime())) return event.starts_at;
  const startText = start.toLocaleString();
  if (!event.ends_at) return startText;
  const end = new Date(event.ends_at);
  if (Number.isNaN(end.getTime())) return startText;
  return `${startText} – ${end.toLocaleString()}`;
}

// The Occasions page's event list (docs/adrs/0026-occasions-events.md, issue
// #1228). Attendee/RSVP management is opened from each row.
export default function OccasionEventList({
  events,
  onEdit,
  onDelete,
  onManageAttendees,
}: OccasionEventListProps) {
  const { t } = useTranslation();

  const handleDeleteClick = (id: string) => {
    if (window.confirm(t('occasions.events.deleteMessage'))) {
      onDelete(id);
    }
  };

  if (events.length === 0) {
    return (
      <Typography variant="body2" sx={{ color: 'text.secondary', py: 2, textAlign: 'center' }}>
        {t('occasions.events.empty')}
      </Typography>
    );
  }

  return (
    <Stack spacing={1.5}>
      {events.map((event) => (
        <Paper key={event.id} variant="outlined" sx={{ p: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body1" sx={{ fontWeight: 500, overflowWrap: 'anywhere' }}>
                {event.title}
              </Typography>
              <Typography variant="body2" sx={{ color: 'text.secondary', mt: 0.5 }}>
                {formatEventWhen(event)}
              </Typography>
              {event.location && (
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                  {event.location}
                </Typography>
              )}
              {event.sensitivity !== 'normal' && (
                <Chip
                  size="small"
                  sx={{ mt: 0.5, height: 18 }}
                  label={t(`occasions.obligation.sensitivities.${event.sensitivity}`)}
                />
              )}
            </Box>
            <Box sx={{ display: 'flex', gap: 0.5 }}>
              <Button
                size="small"
                startIcon={<GroupIcon fontSize="small" />}
                onClick={() => onManageAttendees(event)}
              >
                {t('occasions.events.manageAttendees')}
              </Button>
              <IconButton size="small" onClick={() => onEdit(event)} aria-label={t('common.edit')}>
                <EditIcon fontSize="small" />
              </IconButton>
              <IconButton
                size="small"
                color="error"
                onClick={() => handleDeleteClick(event.id)}
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
