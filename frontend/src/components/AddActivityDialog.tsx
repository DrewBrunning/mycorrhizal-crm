import {
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type Activity,
  addContactToActivities,
  getActivities,
  uploadActivityImage,
} from '../api/activities';
import { type Contact, getContacts } from '../api/contacts';
import { useDiscardGuard } from '../hooks/useDiscardGuard';
import { readSessionDraft, useSessionDraft } from '../hooks/useSessionDraft';
import { endDateFieldError } from '../utils/activityRange';
import { todayLocalISODate } from '../utils/localDate';
import AppDialog from './AppDialog';
import ConfirmDiscardDialog from './ConfirmDiscardDialog';
import DateField from './DateField';

// ActivityPrefill pre-fills the dialog from outside — e.g. confirming a GeoPulse
// location-history suggestion (issue #160). `externalRef` rides through to onSave
// untouched (it is not an editable field). A prefilled dialog never reads or
// writes a session draft: the prefill, not a half-typed draft, is its start state.
export interface ActivityPrefill {
  location?: string;
  // YYYY-MM-DD, the value a date input takes.
  date?: string;
  externalRef?: string;
}

interface AddActivityDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (activity: {
    title: string;
    description: string;
    location: string;
    date: string;
    // Issue #2006: the picked last day (YYYY-MM-DD), only when one is set.
    end_date?: string;
    contact_ids: number[];
    external_ref?: string;
  }) => Promise<Activity | undefined>;
  preselectedContactId?: number;
  prefill?: ActivityPrefill;
  // Called after "Add to existing" changed at least one activity, so the
  // caller can refresh whatever lists the contact's activities.
  onAddedToExisting?: () => void;
}

type DialogMode = 'new' | 'existing';

interface ActivityDraft {
  title: string;
  description: string;
  location: string;
  date: string;
  endDate?: string;
}

export default function AddActivityDialog({
  open,
  onClose,
  onSave,
  preselectedContactId,
  prefill,
  onAddedToExisting,
}: AddActivityDialogProps) {
  const { t } = useTranslation();
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [location, setLocation] = useState('');
  const [date, setDate] = useState(todayLocalISODate());
  // Issue #2006: optional last day of a multi-day activity ('' = single day).
  const [endDate, setEndDate] = useState('');
  // A save refused by the server on the end_date field (shown on that field).
  const [endDateError, setEndDateError] = useState('');
  const [selectedContacts, setSelectedContacts] = useState<Contact[]>([]);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [searchInput, setSearchInput] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(false);
  // ADR 0040 slice (a): images are uploaded after the activity exists, so the
  // dialog keeps the picked files until onSave returns the created activity.
  const [selectedImages, setSelectedImages] = useState<File[]>([]);
  const imageInputRef = useRef<HTMLInputElement>(null);

  // Issue #2007: from a contact's page the dialog can instead add that contact
  // to existing activities. The new-activity state above is left untouched while
  // in this mode, so the draft and the discard guard keep covering it.
  const [mode, setMode] = useState<DialogMode>('new');
  const canAddToExisting = preselectedContactId !== undefined && !prefill;
  const [existingSearch, setExistingSearch] = useState('');
  const [existingActivities, setExistingActivities] = useState<Activity[]>([]);
  const [existingLoading, setExistingLoading] = useState(false);
  const [selectedActivityIds, setSelectedActivityIds] = useState<number[]>([]);
  const [existingError, setExistingError] = useState('');

  // Issue #557: scoped per preselected contact so a draft started from one
  // contact's page never bleeds into another's. The selected-contacts list
  // itself is deliberately not part of the draft -- when preselectedContactId
  // is set, it's loaded async and would otherwise race a restored draft and
  // silently clobber it; title/description/location are the fields with real
  // loss cost anyway.
  const draftKey = `activity-dialog:${preselectedContactId ?? 'unassigned'}`;

  // Primitives, not the prefill object, drive the effect: a parent that builds a
  // fresh object each render must not re-apply the prefill over the user's edits.
  const hasPrefill = prefill !== undefined;
  const prefillLocation = prefill?.location;
  const prefillDate = prefill?.date;
  useEffect(() => {
    if (!open) return;
    if (hasPrefill) {
      if (prefillLocation !== undefined) setLocation(prefillLocation);
      if (prefillDate) setDate(prefillDate);
      return;
    }
    const draft = readSessionDraft<ActivityDraft>(draftKey);
    if (!draft) return;
    setTitle(draft.title);
    setDescription(draft.description);
    setLocation(draft.location);
    setDate(draft.date);
    setEndDate(draft.endDate ?? '');
  }, [open, draftKey, hasPrefill, prefillLocation, prefillDate]);

  // With a prefill the location is already filled in, so it alone is not "work
  // the user would lose" — only what they typed themselves is.
  const isDirty =
    (hasPrefill
      ? Boolean(title.trim() || description.trim())
      : Boolean(title.trim() || description.trim() || location.trim())) ||
    selectedActivityIds.length > 0 ||
    selectedImages.length > 0;
  const { clearDraft } = useSessionDraft(
    draftKey,
    { title, description, location, date, endDate },
    open && isDirty && !hasPrefill,
  );
  // Clears the draft too -- reached after a successful save and after a
  // confirmed discard (a Cancel/Escape close or a blocked in-app navigation),
  // and in every case the draft is no longer wanted.
  const handleClose = useCallback(() => {
    clearDraft();
    setTitle('');
    setDescription('');
    setLocation('');
    setDate(todayLocalISODate());
    setEndDate('');
    setEndDateError('');
    setSelectedContacts([]);
    setSearchInput('');
    setContacts([]);
    setError('');
    setMode('new');
    setExistingSearch('');
    setExistingActivities([]);
    setSelectedActivityIds([]);
    setExistingError('');
    setSelectedImages([]);
    onClose();
  }, [clearDraft, onClose]);
  // Issue #557 + #805: one discard guard covers every way of leaving a dirty
  // activity -- Cancel/Escape (guardedClose), tab close/reload/external nav
  // (beforeunload), and in-app route navigation (the data-router blocker,
  // which discards through handleClose before letting the navigation go).
  const { guardedClose, confirmDialogProps, navigationGuardElement } = useDiscardGuard(isDirty, {
    onNavigationDiscard: handleClose,
  });

  const loadContacts = useCallback(async (search: string = '') => {
    setLoading(true);
    try {
      const response = await getContacts({ limit: 100, search });
      setContacts(response.contacts || []);
    } catch (err) {
      console.error('Failed to fetch contacts:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  // Load preselected contact on open
  useEffect(() => {
    if (open && preselectedContactId) {
      const loadPreselected = async () => {
        try {
          const response = await getContacts({ limit: 100 });
          const preselected = response.contacts.find((c: Contact) => c.ID === preselectedContactId);
          if (preselected) {
            setSelectedContacts([preselected]);
          }
          setContacts(response.contacts || []);
        } catch (err) {
          console.error('Failed to load preselected contact:', err);
        }
      };
      void loadPreselected();
    } else if (open) {
      void loadContacts('');
    }
  }, [open, preselectedContactId, loadContacts]);

  // Debounced search effect
  useEffect(() => {
    if (!open) return;

    const timeoutId = setTimeout(() => {
      void loadContacts(searchInput);
    }, 300);

    return () => clearTimeout(timeoutId);
  }, [searchInput, open, loadContacts]);

  // Existing activities, searchable by title (the API also matches description
  // and location); the server orders them by most recently updated first.
  useEffect(() => {
    if (!open || mode !== 'existing') return;
    let cancelled = false;
    const timeoutId = setTimeout(() => {
      setExistingLoading(true);
      getActivities({ limit: 50, search: existingSearch })
        .then((response) => {
          if (!cancelled) setExistingActivities(response.activities || []);
        })
        .catch((err) => {
          console.error('Failed to fetch activities:', err);
          if (!cancelled) setExistingError(t('activityDialog.existingLoadFailed'));
        })
        .finally(() => {
          if (!cancelled) setExistingLoading(false);
        });
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(timeoutId);
    };
  }, [open, mode, existingSearch, t]);

  const toggleActivity = (id: number) =>
    setSelectedActivityIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    );

  const handleAddToExisting = async () => {
    if (preselectedContactId === undefined) return;
    setSaving(true);
    setExistingError('');
    try {
      const response = await addContactToActivities(preselectedContactId, selectedActivityIds);
      const failed = response.results.filter((r) => r.status === 'not_found');
      if (response.results.some((r) => r.status === 'added')) onAddedToExisting?.();
      if (failed.length === 0) {
        handleClose();
        return;
      }
      // Keep the dialog open on just the failures so they can be retried or dismissed.
      const names = failed.map(
        (f) =>
          existingActivities.find((a) => a.ID === f.activity_id)?.title ?? String(f.activity_id),
      );
      setSelectedActivityIds(failed.map((f) => f.activity_id));
      setExistingError(t('activityDialog.existingPartialFailure', { names: names.join(', ') }));
    } catch {
      setExistingError(t('activityDialog.existingSaveFailed'));
    } finally {
      setSaving(false);
    }
  };

  // Moving the start past an already-picked end leaves the range reversed; the
  // end field says so instead of silently clearing it (issue #2006).
  const endBeforeStart = endDate !== '' && date !== '' && endDate < date;

  const handleSave = async () => {
    if (!title.trim() || !date) {
      setError(t('activityDialog.required'));
      return;
    }

    setSaving(true);
    try {
      const created = await onSave({
        title,
        description,
        location,
        date,
        ...(endDate ? { end_date: endDate } : {}),
        contact_ids: selectedContacts.map((c) => c.ID),
        ...(prefill?.externalRef ? { external_ref: prefill.externalRef } : {}),
      });
      if (created && selectedImages.length > 0) {
        for (const file of selectedImages) {
          await uploadActivityImage(created.ID, file);
        }
      }
      handleClose();
    } catch (err) {
      const fieldError = endDateFieldError(err);
      if (fieldError) setEndDateError(fieldError);
      else setError('Failed to save activity');
    } finally {
      setSaving(false);
    }
  };

  // Issue #557: Cancel and Escape (AppDialog already blocks a backdrop
  // click for every dialog) go through the discard guard -- a dirty
  // activity gets a confirmation instead of vanishing.
  const handleRequestClose = () => guardedClose(handleClose);

  const getContactLabel = (contact: Contact) => {
    return `${contact.firstname}${contact.nickname ? ` "${contact.nickname}"` : ''} ${contact.lastname}`;
  };

  return (
    <AppDialog open={open} onClose={handleRequestClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('activityDialog.title')}</DialogTitle>
      <DialogContent>
        {canAddToExisting && (
          <ToggleButtonGroup
            exclusive
            fullWidth
            size="small"
            value={mode}
            onChange={(_, next: DialogMode | null) => next && setMode(next)}
            aria-label={t('activityDialog.modeLabel')}
            sx={{ mb: 1 }}
          >
            <ToggleButton value="new">{t('activityDialog.modeNew')}</ToggleButton>
            <ToggleButton value="existing">{t('activityDialog.modeExisting')}</ToggleButton>
          </ToggleButtonGroup>
        )}
        {mode === 'existing' ? (
          <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 1 }}>
            <TextField
              label={t('activityDialog.searchExisting')}
              value={existingSearch}
              onChange={(e) => setExistingSearch(e.target.value)}
              fullWidth
            />
            {existingError && (
              <Typography color="error" role="alert" variant="body2">
                {existingError}
              </Typography>
            )}
            {existingLoading && <CircularProgress size={20} />}
            {!existingLoading && existingActivities.length === 0 && (
              <Typography color="text.secondary" variant="body2">
                {t('activityDialog.noExisting')}
              </Typography>
            )}
            <Box
              sx={{
                maxHeight: 320,
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column',
              }}
            >
              {existingActivities.map((a) => (
                <FormControlLabel
                  key={a.ID}
                  control={
                    <Checkbox
                      checked={selectedActivityIds.includes(a.ID)}
                      onChange={() => toggleActivity(a.ID)}
                    />
                  }
                  label={`${a.title} · ${a.date.slice(0, 10)}`}
                />
              ))}
            </Box>
          </Box>
        ) : (
          <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
            <TextField
              label={t('activityDialog.activityTitle')}
              placeholder={t('activityDialog.titlePlaceholder')}
              value={title}
              onChange={(e) => {
                setTitle(e.target.value);
                setError('');
              }}
              error={!!error}
              helperText={error}
              fullWidth
              required
              autoFocus
            />

            <TextField
              label={t('activityDialog.description')}
              placeholder={t('activityDialog.descriptionPlaceholder')}
              multiline
              rows={3}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              fullWidth
            />

            <TextField
              label={t('activityDialog.location')}
              placeholder={t('activityDialog.locationPlaceholder')}
              value={location}
              onChange={(e) => setLocation(e.target.value)}
              fullWidth
            />

            <DateField
              label={t('activityDialog.date')}
              value={date}
              onChange={(value) => {
                setDate(value);
                setEndDateError('');
              }}
              required
            />
            <DateField
              label={t('activityDialog.endDate')}
              value={endDate}
              onChange={(value) => {
                setEndDate(value);
                setEndDateError('');
              }}
              min={date || undefined}
              errorText={endDateError || (endBeforeStart ? t('activityDialog.endBeforeStart') : '')}
            />

            <Autocomplete
              multiple
              options={contacts}
              getOptionLabel={getContactLabel}
              value={selectedContacts}
              onChange={(_, newValue) => setSelectedContacts(newValue)}
              onInputChange={(_, value) => setSearchInput(value)}
              inputValue={searchInput}
              loading={loading}
              filterOptions={(x) => x}
              isOptionEqualToValue={(option, value) => option.ID === value.ID}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label={t('activityDialog.contacts')}
                  placeholder={t('activityDialog.selectContacts')}
                  slotProps={{
                    ...params.slotProps,

                    input: {
                      ...params.slotProps.input,
                      endAdornment: (
                        <>
                          {loading ? <CircularProgress color="inherit" size={20} /> : null}
                          {params.slotProps.input.endAdornment}
                        </>
                      ),
                    },
                  }}
                />
              )}
              renderValue={(value, getItemProps) =>
                value.map((contact, index) => (
                  <Chip
                    label={getContactLabel(contact)}
                    {...getItemProps({ index })}
                    key={contact.ID}
                  />
                ))
              }
            />
          </Box>
        )}
      </DialogContent>
      <Box sx={{ mt: 1 }}>
        <input
          ref={imageInputRef}
          type="file"
          aria-label={t('activityImages.add')}
          accept="image/jpeg,image/png,image/gif"
          multiple
          hidden
          onChange={(event) =>
            setSelectedImages((prev) => [...prev, ...Array.from(event.target.files ?? [])])
          }
        />
        <Button size="small" onClick={() => imageInputRef.current?.click()}>
          {t('activityImages.add')}
        </Button>
        {selectedImages.length > 0 && (
          <Typography variant="caption" sx={{ ml: 1, color: 'text.secondary' }}>
            {t('activityImages.queued', { count: selectedImages.length })}
          </Typography>
        )}
      </Box>
      <DialogActions>
        <Button onClick={handleRequestClose} disabled={saving}>
          {t('activityDialog.cancel')}
        </Button>
        {mode === 'existing' ? (
          <Button
            onClick={() => void handleAddToExisting()}
            variant="contained"
            disabled={saving || selectedActivityIds.length === 0}
          >
            {t('activityDialog.addToSelected')}
          </Button>
        ) : (
          <Button
            onClick={() => void handleSave()}
            variant="contained"
            disabled={saving || endBeforeStart}
          >
            {t('activityDialog.save')}
          </Button>
        )}
      </DialogActions>
      {navigationGuardElement}
      <ConfirmDiscardDialog {...confirmDialogProps} />
    </AppDialog>
  );
}
