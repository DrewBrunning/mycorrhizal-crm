import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import {
  Autocomplete,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Circle, listCircles } from '../api/circles';
import {
  OCCASION_EVENT_RSVPS,
  type OccasionEvent,
  type OccasionEventRSVP,
} from '../api/occasionEvents';
import { useOccasionEventAttendees } from '../hooks/useOccasionEvents';
import AppDialog from './AppDialog';

interface OccasionEventAttendeesDialogProps {
  open: boolean;
  onClose: () => void;
  event: OccasionEvent | null;
}

// The attendee/RSVP panel for one event (docs/adrs/0026-occasions-events.md,
// issue #1228). The RSVP recorded here is what the user reports the contact
// told them — no invitation is sent, and the helper text says so.
export default function OccasionEventAttendeesDialog({
  open,
  onClose,
  event,
}: OccasionEventAttendeesDialogProps) {
  const { t } = useTranslation();
  const {
    attendees,
    loading,
    error,
    handleAdd,
    handleUpdateRsvp,
    handleRemove,
    suggestions,
    suggestionsLoading,
    loadSuggestions,
    clearSuggestions,
  } = useOccasionEventAttendees(open ? (event?.id ?? undefined) : undefined);

  const [circles, setCircles] = useState<Circle[]>([]);
  const [selectedCircleIds, setSelectedCircleIds] = useState<string[]>([]);
  const [actionError, setActionError] = useState('');

  useEffect(() => {
    if (!open) return;
    setSelectedCircleIds([]);
    setActionError('');
    clearSuggestions();
    listCircles({ limit: 100 })
      .then((response) => setCircles(response.circles || []))
      .catch(() => setCircles([]));
  }, [open, clearSuggestions]);

  const handleSuggest = async () => {
    setActionError('');
    await loadSuggestions(selectedCircleIds);
  };

  const handleAddSuggestion = async (entityId: string) => {
    setActionError('');
    try {
      await handleAdd(entityId);
      clearSuggestions();
    } catch {
      setActionError(t('occasions.events.attendeeSaveFailed'));
    }
  };

  const handleRsvpChange = async (vcardUid: string, rsvp: OccasionEventRSVP) => {
    setActionError('');
    try {
      await handleUpdateRsvp(vcardUid, rsvp);
    } catch {
      setActionError(t('occasions.events.attendeeSaveFailed'));
    }
  };

  const handleRemoveAttendee = async (vcardUid: string) => {
    setActionError('');
    try {
      await handleRemove(vcardUid);
    } catch {
      setActionError(t('occasions.events.attendeeSaveFailed'));
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {t('occasions.events.attendeesTitle', { title: event?.title ?? '' })}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {t('occasions.events.rsvpHint')}
          </Typography>

          {error && (
            <Typography color="error" variant="body2">
              {error}
            </Typography>
          )}

          {loading && attendees.length === 0 ? (
            <Typography variant="body2">{t('common.loading')}</Typography>
          ) : attendees.length === 0 ? (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('occasions.events.noAttendees')}
            </Typography>
          ) : (
            <Stack spacing={1} divider={<Divider flexItem />}>
              {attendees.map((attendee) => (
                <Box key={attendee.id} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Typography variant="body2" sx={{ flexGrow: 1, overflowWrap: 'anywhere' }}>
                    {attendee.contact_name}
                  </Typography>
                  <TextField
                    select
                    size="small"
                    label={t('occasions.events.rsvpLabel')}
                    value={attendee.rsvp}
                    onChange={(e) =>
                      void handleRsvpChange(attendee.entity_id, e.target.value as OccasionEventRSVP)
                    }
                    sx={{ width: 140 }}
                  >
                    {OCCASION_EVENT_RSVPS.map((rsvp) => (
                      <MenuItem key={rsvp} value={rsvp}>
                        {t(`occasions.events.rsvp.${rsvp}`)}
                      </MenuItem>
                    ))}
                  </TextField>
                  <IconButton
                    size="small"
                    color="error"
                    aria-label={t('occasions.events.removeAttendee')}
                    onClick={() => void handleRemoveAttendee(attendee.entity_id)}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </Box>
              ))}
            </Stack>
          )}

          <Divider />

          <Typography variant="subtitle2">{t('occasions.events.suggestFromCircles')}</Typography>
          <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start' }}>
            <Autocomplete
              multiple
              options={circles}
              getOptionLabel={(option) => option.name}
              value={circles.filter((c) => selectedCircleIds.includes(c.id))}
              onChange={(_e, value) => {
                setSelectedCircleIds(value.map((c) => c.id));
                clearSuggestions();
              }}
              renderInput={(params) => (
                <TextField {...params} label={t('occasions.events.selectCircles')} size="small" />
              )}
              sx={{ flexGrow: 1 }}
            />
            <Button
              variant="outlined"
              onClick={() => void handleSuggest()}
              disabled={selectedCircleIds.length === 0 || suggestionsLoading}
            >
              {t('occasions.events.suggest')}
            </Button>
          </Box>

          {!suggestionsLoading && suggestions.length > 0 && (
            <Stack spacing={1}>
              {suggestions.map((suggestion) => (
                <Box
                  key={suggestion.entity_id}
                  sx={{ display: 'flex', alignItems: 'center', gap: 1 }}
                >
                  <Typography variant="body2" sx={{ flexGrow: 1 }}>
                    {suggestion.contact_name}
                  </Typography>
                  <Button
                    size="small"
                    startIcon={<AddIcon fontSize="small" />}
                    onClick={() => void handleAddSuggestion(suggestion.entity_id)}
                  >
                    {t('occasions.events.addAttendee')}
                  </Button>
                </Box>
              ))}
            </Stack>
          )}

          {selectedCircleIds.length > 0 &&
            !suggestionsLoading &&
            suggestions.length === 0 &&
            !actionError && (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('occasions.events.noSuggestions')}
              </Typography>
            )}

          {actionError && (
            <Typography color="error" variant="body2">
              {actionError}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.close')}</Button>
      </DialogActions>
    </AppDialog>
  );
}
