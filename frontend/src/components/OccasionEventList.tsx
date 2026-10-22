import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import GroupIcon from '@mui/icons-material/Group';
import { Box, Button, Chip, IconButton, Paper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { OccasionEvent } from '../api/occasionEvents';
import {
  formatDateTime as formatDateTimeDefault,
  formatTime as formatTimeDefault,
  useFormatDateTime,
  useTimeFormat,
} from '../timeFormat';
import { localISODate } from '../utils/localDate';

interface OccasionEventListProps {
  events: OccasionEvent[];
  onEdit: (event: OccasionEvent) => void;
  onDelete: (id: string) => void;
  onManageAttendees: (event: OccasionEvent) => void;
}

// formatEventWhen renders the start (and optional end) in the user's date and
// time preferences. Events are moments in time (RFC 3339), so the local zone
// is correct here (#1753 parity with Android).
//  - no end, or end == start (the same instant): just the start;
//  - ends the same local day: `<date> <start> – <end time>`;
//  - otherwise: the full start and the full end.
export function formatEventWhen(
  event: OccasionEvent,
  formatDateTime: (value: string | number | Date) => string = formatDateTimeDefault,
  formatTime: (value: string | number | Date) => string = formatTimeDefault,
): string {
  const start = new Date(event.starts_at);
  if (Number.isNaN(start.getTime())) return event.starts_at;
  const startText = formatDateTime(start);
  const endIso = event.ends_at?.trim();
  if (!endIso) return startText;
  const end = new Date(endIso);
  if (Number.isNaN(end.getTime())) return startText;
  if (end.getTime() === start.getTime()) return startText;
  if (localISODate(start) === localISODate(end)) {
    return `${startText} – ${formatTime(end)}`;
  }
  return `${startText} – ${formatDateTime(end)}`;
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
  const formatDateTime = useFormatDateTime();
  const { formatTime } = useTimeFormat();

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
          <Box
            sx={{
              display: 'flex',
              // At 320px the content and the three actions cannot share a row:
              // the event text used to stack one word per line. Stack them
              // instead of squeezing the content (#1789).
              flexDirection: { xs: 'column', sm: 'row' },
              justifyContent: 'space-between',
              alignItems: { xs: 'stretch', sm: 'flex-start' },
              gap: 1,
            }}
          >
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body1" sx={{ fontWeight: 500, overflowWrap: 'anywhere' }}>
                {event.title}
              </Typography>
              <Typography variant="body2" sx={{ color: 'text.secondary', mt: 0.5 }}>
                {formatEventWhen(event, formatDateTime, formatTime)}
              </Typography>
              {event.location && (
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                  {event.location}
                </Typography>
              )}
              {event.sensitivity !== 'normal' && (
                <Chip
                  size="small"
                  sx={{ mt: 0.5 }}
                  label={t(`occasions.obligation.sensitivities.${event.sensitivity}`)}
                />
              )}
            </Box>
            <Box
              sx={{
                display: 'flex',
                gap: 0.5,
                flexWrap: 'wrap',
                justifyContent: { xs: 'flex-start', sm: 'flex-end' },
              }}
            >
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
