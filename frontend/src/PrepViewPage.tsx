import { useEffect, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Chip,
  Avatar,
  CircularProgress,
  Alert,
  Button,
  Divider,
  Stack,
  SvgIcon,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import WarningIcon from '@mui/icons-material/Warning';
import EventIcon from '@mui/icons-material/Event';
import FavoriteIcon from '@mui/icons-material/Favorite';
import ChatIcon from '@mui/icons-material/Chat';
import CakeIcon from '@mui/icons-material/Cake';
import CelebrationIcon from '@mui/icons-material/Celebration';
import { mdiNoteMultipleOutline } from '@mdi/js';
import { getContactBriefing, ContactBriefing, BriefingRelationship } from './api/briefings';
import { useDateFormat } from './DateFormatProvider';
import { handleFetchError } from './utils/errorHandler';
import { useSnackbar } from './context/SnackbarContext';

// N2 prep view (docs/fork-plan/tickets/22-N2-prep-view.md): the "person
// briefing" — everything the user wants to remember in the five minutes
// before seeing someone, scannable in under a minute. Read-only: a pure
// composition of data the contact page already surfaces, assembled server-side
// by GET /contacts/:id/briefing.
//
// Every block degrades gracefully when its source is empty — a contact with
// no activities, no agenda, no cadence policy, no relationships renders the
// same layout with each card showing its empty state.
export default function PrepViewPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { showError } = useSnackbar();
  const { formatDate, formatBirthday } = useDateFormat();
  const [briefing, setBriefing] = useState<ContactBriefing | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    (async () => {
      try {
        const data = await getContactBriefing(id);
        if (!cancelled) setBriefing(data);
      } catch (err) {
        if (!cancelled) {
          const msg = handleFetchError(err, 'loading prep view');
          setError(msg);
          showError(msg);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [id, showError]);

  if (loading) {
    return (
      <Box sx={{ maxWidth: 900, mx: 'auto', mt: 2, px: 2, pb: 2 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error || !briefing) {
    return (
      <Box sx={{ maxWidth: 900, mx: 'auto', mt: 2, px: 2, pb: 2 }}>
        <Alert severity="error">{error || t('prep.notFound')}</Alert>
      </Box>
    );
  }

  const relationshipLabel = (r: BriefingRelationship): string =>
    r.display_token ? t(`relationships.types.${r.display_token}`, r.display_token) : t('relationships.types.related_to');

  return (
    <Box sx={{ maxWidth: 900, mx: 'auto', mt: 1, px: 2, pb: 2 }}>
      <Button
        startIcon={<ArrowBackIcon />}
        onClick={() => navigate(`/contacts/${briefing.contact_id}`)}
        size="small"
        sx={{ mb: 1.5 }}
      >
        {t('common.back')}
      </Button>

      {/* Header */}
      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: 1.5, display: 'flex', alignItems: 'center', gap: 2 }}>
          <Avatar src={briefing.photo_thumbnail || undefined} sx={{ width: 56, height: 56, bgcolor: 'primary.main' }}>
            {(briefing.name || '?').charAt(0).toUpperCase()}
          </Avatar>
          <Box sx={{ flexGrow: 1, minWidth: 0 }}>
            <Typography variant="h5" sx={{ fontWeight: 500, overflowWrap: 'anywhere' }}>
              {briefing.name || t('prep.unknownContact')}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {t('prep.subtitle')}
            </Typography>
          </Box>
          {briefing.kind === 'animal' && (
            <Chip label={t('contactDetail.animal')} size="small" color="secondary" />
          )}
        </CardContent>
      </Card>

      {/* Cadence health — "how overdue is this relationship" */}
      {briefing.cadence && (
        <Card sx={{ mb: 2, border: '1px solid', borderColor: briefing.cadence.health.overdue_by > 0 ? 'warning.main' : 'divider' }}>
          <CardContent sx={{ py: 1.5 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <WarningIcon fontSize="small" color={briefing.cadence.health.overdue_by > 0 ? 'warning' : 'disabled'} />
              <Typography variant="subtitle2">{t('prep.cadence.title')}</Typography>
            </Stack>
            {briefing.cadence.health.has_qualifying_interaction ? (
              <Box sx={{ mt: 0.5 }}>
                {briefing.cadence.health.overdue_by > 0 ? (
                  <Typography variant="body2" color="warning.main">
                    {t('cadence.overdueBy', { days: briefing.cadence.health.overdue_by })}
                  </Typography>
                ) : (
                  <Typography variant="body2" color="success.main">
                    {t('cadence.onTrack')}
                  </Typography>
                )}
                {briefing.cadence.health.next_due && (
                  <Typography variant="caption" color="text.secondary">
                    {t('cadence.nextDue', { date: formatDate(briefing.cadence.health.next_due) })}
                  </Typography>
                )}
                {briefing.cadence.health.last_interaction && (
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                    {t('cadence.lastInteraction', { date: formatDate(briefing.cadence.health.last_interaction) })}
                  </Typography>
                )}
              </Box>
            ) : (
              <Typography variant="body2" color="text.secondary">
                {t('cadence.noInteractionsYet')}
              </Typography>
            )}
          </CardContent>
        </Card>
      )}

      {/* Open agenda items — "things to bring up" */}
      {briefing.open_agenda_items.length > 0 && (
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <ChatIcon fontSize="small" color="action" />
              <Typography variant="subtitle2">{t('prep.agenda.title')}</Typography>
            </Stack>
            <Stack spacing={0.5} sx={{ mt: 0.5 }}>
              {briefing.open_agenda_items.map((item) => (
                <Typography key={item.id} variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                  • {item.content}
                </Typography>
              ))}
            </Stack>
          </CardContent>
        </Card>
      )}

      {/* Last interaction + recent notes — "what happened between us" */}
      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: 1.5 }}>
          <Stack direction="row" alignItems="center" spacing={1}>
            <EventIcon fontSize="small" color="action" />
            <Typography variant="subtitle2">{t('prep.lastInteraction.title')}</Typography>
          </Stack>
          {briefing.last_activity ? (
            <Box sx={{ mt: 0.5 }}>
              <Typography variant="body2" fontWeight={500}>
                {briefing.last_activity.title}
                {briefing.last_activity.type ? ` (${briefing.last_activity.type})` : ''}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {formatDate(briefing.last_activity.date)}
              </Typography>
              {briefing.last_activity.description && (
                <Typography variant="body2" sx={{ mt: 0.5, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
                  {briefing.last_activity.description}
                </Typography>
              )}
            </Box>
          ) : (
            <Typography variant="body2" color="text.secondary">
              {t('prep.lastInteraction.none')}
            </Typography>
          )}

          {briefing.recent_notes.length > 0 && (
            <>
              <Divider sx={{ my: 1 }} />
              <Stack direction="row" alignItems="center" spacing={1}>
                <SvgIcon fontSize="small" color="action"><path d={mdiNoteMultipleOutline} /></SvgIcon>
                <Typography variant="subtitle2">{t('prep.recentNotes.title')}</Typography>
              </Stack>
              <Stack spacing={0.5} sx={{ mt: 0.5 }}>
                {briefing.recent_notes.map((note) => (
                  <Typography key={note.ID} variant="body2" sx={{ overflowWrap: 'anywhere', whiteSpace: 'pre-wrap' }}>
                    • {note.content}
                  </Typography>
                ))}
              </Stack>
            </>
          )}
        </CardContent>
      </Card>

      {/* Relationships — partner's and kids' names */}
      {briefing.relationships.length > 0 && (
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <FavoriteIcon fontSize="small" color="action" />
              <Typography variant="subtitle2">{t('prep.relationships.title')}</Typography>
            </Stack>
            <Stack spacing={0.5} sx={{ mt: 0.5 }}>
              {briefing.relationships.map((r) => (
                <Box key={r.edge.id} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Typography variant="body2">
                    {r.other_party_name || t('prep.unknownContact')}
                    {r.display_token ? ` — ${relationshipLabel(r)}` : ''}
                  </Typography>
                  {r.other_party_contact_id && (
                    <Link to={`/contacts/${r.other_party_contact_id}`} style={{ display: 'inline-flex' }}>
                      <Chip label={t('prep.view')} size="small" variant="outlined" />
                    </Link>
                  )}
                </Box>
              ))}
            </Stack>
          </CardContent>
        </Card>
      )}

      {/* Life events */}
      {briefing.life_events.length > 0 && (
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <EventIcon fontSize="small" color="action" />
              <Typography variant="subtitle2">{t('prep.lifeEvents.title')}</Typography>
            </Stack>
            <Stack spacing={0.5} sx={{ mt: 0.5 }}>
              {briefing.life_events.map((event) => (
                <Typography key={event.id} variant="body2">
                  • {t(`lifeEvent.types.${event.type}`, event.type)}
                  {event.description ? ` — ${event.description}` : ''}
                </Typography>
              ))}
            </Stack>
          </CardContent>
        </Card>
      )}

      {/* Upcoming reminders */}
      {briefing.upcoming_reminders.length > 0 && (
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <EventIcon fontSize="small" color="action" />
              <Typography variant="subtitle2">{t('prep.reminders.title')}</Typography>
            </Stack>
            <Stack spacing={0.5} sx={{ mt: 0.5 }}>
              {briefing.upcoming_reminders.map((reminder) => (
                <Typography key={reminder.ID} variant="body2">
                  • {reminder.message}
                  {' '}
                  <Typography component="span" variant="caption" color="text.secondary">
                    ({formatDate(reminder.remind_at)})
                  </Typography>
                </Typography>
              ))}
            </Stack>
          </CardContent>
        </Card>
      )}

      {/* Upcoming dates — birthday, anniversary */}
      {briefing.upcoming_dates.length > 0 && (
        <Card>
          <CardContent sx={{ py: 1.5 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <CakeIcon fontSize="small" color="action" />
              <Typography variant="subtitle2">{t('prep.upcomingDates.title')}</Typography>
            </Stack>
            <Stack spacing={0.5} sx={{ mt: 0.5 }}>
              {briefing.upcoming_dates.map((d, i) => (
                <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  {d.label === 'birthday' ? (
                    <CakeIcon fontSize="small" color="action" />
                  ) : (
                    <CelebrationIcon fontSize="small" color="action" />
                  )}
                  <Typography variant="body2">
                    {t(`prep.upcomingDates.${d.label}`)}
                    {' '}
                    {formatBirthday(d.date)}
                  </Typography>
                  <Chip
                    label={t('prep.upcomingDates.inDays', { days: d.days_until })}
                    size="small"
                    variant="outlined"
                  />
                </Box>
              ))}
            </Stack>
          </CardContent>
        </Card>
      )}
    </Box>
  );
}
