import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Link,
  Paper,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { createActivity } from '../api/activities';
import type { GeoPulseStaySuggestion } from '../api/geopulse';
import { useGeoPulse } from '../hooks/useGeoPulse';
import AddActivityDialog from './AddActivityDialog';
import AppDialog from './AppDialog';

interface GeoPulseSuggestionsDialogProps {
  open: boolean;
  onClose: () => void;
  // Called after a suggestion has been confirmed into an Activity, so the host
  // page can refresh its list.
  onLogged?: () => void;
}

// localToday is today's date as a date input expects it (YYYY-MM-DD), in the
// user's own timezone — not toISOString()'s UTC date, which is "tomorrow" in the
// evening east of Greenwich.
function localToday(): string {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
}

function browserTimezone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
  } catch {
    return undefined;
  }
}

// stayLocalDate is the stay's own calendar day (YYYY-MM-DD) in the lookup's
// timezone. The activity form is pre-filled from this, never from the live date
// input, which the user may have changed after the lookup without re-running it.
function stayLocalDate(timestamp: string, timeZone: string | undefined, fallback: string): string {
  const d = new Date(timestamp);
  if (Number.isNaN(d.getTime())) return fallback;
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      timeZone,
    }).formatToParts(d);
    const get = (type: string) => parts.find((p) => p.type === type)?.value;
    const [y, m, day] = [get('year'), get('month'), get('day')];
    return y && m && day ? `${y}-${m}-${day}` : fallback;
  } catch {
    return fallback;
  }
}

function formatStayTime(timestamp: string, timeZone: string | undefined): string {
  try {
    return new Date(timestamp).toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      timeZone,
    });
  } catch {
    return new Date(timestamp).toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
    });
  }
}

// GeoPulseSuggestionsDialog is the "log activity from location history" flow
// (issue #160, ADR 0033): pick a date → see where GeoPulse says you were → pick
// a stay → choose who you were with → confirm. The list is ephemeral (nothing is
// stored until you confirm); photos are shown, never saved; and the contacts are
// always your choice — nothing is inferred from location.
export default function GeoPulseSuggestionsDialog({
  open,
  onClose,
  onLogged,
}: GeoPulseSuggestionsDialogProps) {
  const { t } = useTranslation();
  const geopulse = useGeoPulse();
  const timezone = useMemo(browserTimezone, []);

  const [date, setDate] = useState(localToday);
  const [confirming, setConfirming] = useState<GeoPulseStaySuggestion | null>(null);
  // Refs confirmed during this dialog's lifetime — shown as logged straight away,
  // without a second lookup.
  const [loggedRefs, setLoggedRefs] = useState<Set<string>>(new Set());

  const { clearSuggestions } = geopulse;
  useEffect(() => {
    if (!open) {
      clearSuggestions();
      setConfirming(null);
      setLoggedRefs(new Set());
    }
  }, [open, clearSuggestions]);

  const handleLookup = () => {
    if (!date) return;
    void geopulse.lookup(date, timezone);
  };

  const handleConfirm = useCallback(
    async (activity: {
      title: string;
      description: string;
      location: string;
      date: string;
      contact_ids: number[];
      external_ref?: string;
    }) => {
      await createActivity({ ...activity, date: new Date(activity.date).toISOString() });
      if (activity.external_ref) {
        const ref = activity.external_ref;
        setLoggedRefs((prev) => new Set(prev).add(ref));
      }
      onLogged?.();
    },
    [onLogged],
  );

  const formatDuration = (seconds: number): string => {
    const minutes = Math.max(1, Math.round(seconds / 60));
    if (minutes < 60) return t('geopulse.dialog.durationMinutes', { count: minutes });
    const hours = Math.floor(minutes / 60);
    const rest = minutes % 60;
    return rest === 0
      ? t('geopulse.dialog.durationHours', { count: hours })
      : t('geopulse.dialog.durationHoursMinutes', { hours, minutes: rest });
  };

  const suggestions = geopulse.suggestions?.suggestions ?? [];

  // Polite live-region text: loading, outcome of the lookup, or nothing. The
  // visible error Alert is itself role="alert", so errors are not repeated here.
  let announcement = '';
  if (geopulse.suggestionsLoading) {
    announcement = t('geopulse.dialog.lookingUp');
  } else if (geopulse.suggestions) {
    announcement =
      suggestions.length === 0
        ? t('geopulse.dialog.announceNone')
        : t('geopulse.dialog.announceFound', { count: suggestions.length });
  }

  return (
    <>
      <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
        <DialogTitle>{t('geopulse.dialog.title')}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('geopulse.dialog.description')}
            </Typography>

            <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start' }}>
              <TextField
                label={t('geopulse.dialog.date')}
                type="date"
                value={date}
                onChange={(e) => setDate(e.target.value)}
                size="small"
                fullWidth
                slotProps={{ inputLabel: { shrink: true } }}
              />
              <Button
                variant="contained"
                onClick={handleLookup}
                disabled={!date || geopulse.suggestionsLoading}
                sx={{ whiteSpace: 'nowrap', minHeight: 40 }}
              >
                {geopulse.suggestionsLoading
                  ? t('geopulse.dialog.lookingUp')
                  : t('geopulse.dialog.lookUp')}
              </Button>
            </Box>

            {/* Always mounted so assistive tech registers it before the text changes. */}
            <Box
              role="status"
              aria-live="polite"
              sx={{
                position: 'absolute',
                width: 1,
                height: 1,
                overflow: 'hidden',
                clip: 'rect(0 0 0 0)',
                whiteSpace: 'nowrap',
              }}
            >
              {announcement}
            </Box>

            {geopulse.suggestionsLoading && (
              <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                <CircularProgress size={24} aria-label={t('geopulse.dialog.lookingUp')} />
              </Box>
            )}

            {geopulse.suggestionsError && (
              <Alert severity="error">
                {geopulse.suggestionsError}{' '}
                <Link component={RouterLink} to="/settings" onClick={onClose}>
                  {t('geopulse.dialog.openSettings')}
                </Link>
              </Alert>
            )}

            {geopulse.suggestions && suggestions.length === 0 && (
              <Alert severity="info">{t('geopulse.dialog.noStays')}</Alert>
            )}

            {suggestions.map((s) => {
              const logged = loggedRefs.has(s.external_ref) || s.existing_activity_id !== undefined;
              const place = [s.city, s.country].filter(Boolean).join(', ');
              return (
                <Paper key={s.external_ref} variant="outlined" sx={{ p: 1.5 }}>
                  <Box
                    sx={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'flex-start',
                      gap: 1,
                    }}
                  >
                    <Box sx={{ minWidth: 0 }}>
                      <Typography variant="subtitle2" component="h3">
                        {s.location || t('geopulse.dialog.unnamedPlace')}
                      </Typography>
                      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                        {formatStayTime(s.timestamp, timezone)} ·{' '}
                        {formatDuration(s.duration_seconds)}
                        {place ? ` · ${place}` : ''}
                      </Typography>
                    </Box>
                    {logged ? (
                      <Chip
                        size="small"
                        color="success"
                        label={t('geopulse.dialog.alreadyLogged')}
                      />
                    ) : (
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => setConfirming(s)}
                        aria-label={t('geopulse.dialog.logActivityAt', {
                          place: s.location || t('geopulse.dialog.unnamedPlace'),
                          time: formatStayTime(s.timestamp, timezone),
                        })}
                      >
                        {t('geopulse.dialog.logActivity')}
                      </Button>
                    )}
                  </Box>

                  {s.photos.length > 0 && (
                    <Box sx={{ mt: 1 }}>
                      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t('geopulse.dialog.photos', { count: s.photos.length })}
                      </Typography>
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 0.5 }}>
                        {s.photos.map((p) => (
                          <Chip
                            key={p.id}
                            size="small"
                            variant="outlined"
                            label={p.file_name || p.id}
                          />
                        ))}
                      </Box>
                    </Box>
                  )}
                  {s.photos.length === 0 && s.photos_unavailable && (
                    <Typography
                      variant="caption"
                      sx={{ color: 'text.secondary', display: 'block', mt: 1 }}
                    >
                      {t('geopulse.dialog.photosUnavailable')}
                    </Typography>
                  )}
                </Paper>
              );
            })}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose}>{t('common.close')}</Button>
        </DialogActions>
      </AppDialog>

      {confirming && (
        <AddActivityDialog
          open
          onClose={() => setConfirming(null)}
          onSave={handleConfirm}
          prefill={{
            location: confirming.location,
            date: stayLocalDate(confirming.timestamp, timezone, geopulse.suggestions?.date ?? date),
            externalRef: confirming.external_ref,
          }}
        />
      )}
    </>
  );
}
