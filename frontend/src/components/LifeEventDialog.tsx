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
  MenuItem,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Contact, getContacts, getContactsByUid } from '../api/contacts';
import {
  isKnownLifeEventCategory,
  LIFE_EVENT_CATEGORIES,
  LIFE_EVENT_TYPES_BY_CATEGORY,
  type LifeEventCategory,
  type PartialDate,
  partialDateHasMonthDay,
  partialDateIsYearOnly,
} from '../api/lifeEvents';
import { handleFetchError } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface ContactBrief {
  uid: string;
  firstname: string;
  lastname: string;
  nickname?: string;
}

function toContactBrief(c: Contact): ContactBrief {
  return { uid: c.uid!, firstname: c.firstname, lastname: c.lastname, nickname: c.nickname };
}

function contactLabel(c: ContactBrief): string {
  if (c.nickname) {
    return `${c.firstname} "${c.nickname}" ${c.lastname}`;
  }
  return `${c.firstname} ${c.lastname}`;
}

export interface LifeEventFormData {
  type: string;
  category?: string;
  date?: PartialDate;
  description?: string;
  relatedEntityIds?: string[];
  remind?: boolean;
}

// UNCATEGORIZED is a UI-only sentinel (T36) — never sent to the backend as a
// Category value. It covers two cases: an existing pre-migration life event
// whose Category was left NULL (nothing to backfill it from, see migration
// 000011's own comment), and a brand-new event the user deliberately leaves
// uncategorized. Selecting it renders Type as a plain free-text field, since
// no category-specific default list applies.
const UNCATEGORIZED = 'uncategorized';

// CUSTOM_TYPE sentinel for the trailing "Add a new life event type" option in
// each category's Type select (ticket item 4) — picking it swaps the Select
// for a free-text field instead of submitting the sentinel itself as Type.
const CUSTOM_TYPE = '__custom__';

interface LifeEventDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: LifeEventFormData) => Promise<void>;
  initial?: LifeEventFormData;
  excludeContactUid?: string;
}

export default function LifeEventDialog({
  open,
  onClose,
  onSave,
  initial,
  excludeContactUid,
}: LifeEventDialogProps) {
  const { t } = useTranslation();
  const [category, setCategory] = useState<LifeEventCategory | typeof UNCATEGORIZED | ''>('');
  const [type, setType] = useState('');
  const [useCustomType, setUseCustomType] = useState(false);
  const [dateYear, setDateYear] = useState('');
  const [dateMonth, setDateMonth] = useState('');
  const [dateDay, setDateDay] = useState('');
  const [description, setDescription] = useState('');
  const [remind, setRemind] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const [contacts, setContacts] = useState<ContactBrief[]>([]);
  const [contactsLoading, setContactsLoading] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [relatedContacts, setRelatedContacts] = useState<ContactBrief[]>([]);

  const isEditing = !!initial;

  const loadContacts = useCallback(
    async (search: string = '') => {
      setContactsLoading(true);
      try {
        const response = await getContacts({ limit: 40, search });
        let filtered = (response.contacts || []).filter((c) => c.uid).map(toContactBrief);
        if (excludeContactUid) {
          filtered = filtered.filter((c) => c.uid !== excludeContactUid);
        }
        setContacts(filtered);
      } catch (err) {
        handleFetchError(err, 'loading contacts');
      } finally {
        setContactsLoading(false);
      }
    },
    [excludeContactUid],
  );

  useEffect(() => {
    if (!open || !isEditing) return;
    const timeoutId = setTimeout(() => {
      loadContacts(searchInput);
    }, 300);
    return () => clearTimeout(timeoutId);
  }, [searchInput, open, isEditing, loadContacts]);

  useEffect(() => {
    if (open && !isEditing) {
      loadContacts();
    }
  }, [open, isEditing, loadContacts]);

  useEffect(() => {
    if (open) {
      if (initial) {
        // Falls back to UNCATEGORIZED both for a genuinely empty Category
        // (the legacy/pre-migration case) and for an unrecognized one (stale
        // data, or a category token this frontend copy predates) — isKnownLifeEventCategory
        // guards the LIFE_EVENT_TYPES_BY_CATEGORY lookup below from throwing
        // on the latter.
        const initCategory: LifeEventCategory | typeof UNCATEGORIZED =
          initial.category && isKnownLifeEventCategory(initial.category)
            ? initial.category
            : UNCATEGORIZED;
        setCategory(initCategory);
        setType(initial.type || '');
        // A predefined category's type list decides whether the existing
        // value renders as a picked option or a custom one; UNCATEGORIZED
        // always renders as free text (see the Type field below), so its
        // useCustomType value is never read.
        setUseCustomType(
          initCategory !== UNCATEGORIZED &&
            !LIFE_EVENT_TYPES_BY_CATEGORY[initCategory].includes(initial.type || ''),
        );
        if (initial.date) {
          setDateYear(initial.date.year != null ? String(initial.date.year) : '');
          setDateMonth(initial.date.month != null ? String(initial.date.month) : '');
          setDateDay(initial.date.day != null ? String(initial.date.day) : '');
        } else {
          setDateYear('');
          setDateMonth('');
          setDateDay('');
        }
        setDescription(initial.description || '');
        setRemind(initial.remind || false);
        setRelatedContacts([]);
        // Pre-populate related contacts from their VCardUIDs.
        if (initial.relatedEntityIds && initial.relatedEntityIds.length > 0) {
          getContactsByUid(initial.relatedEntityIds)
            .then((byUid) => {
              const briefs: ContactBrief[] = [];
              byUid.forEach((c) => {
                if (c.uid) briefs.push(toContactBrief(c));
              });
              setRelatedContacts(briefs);
            })
            .catch(() => {});
        }
      } else {
        setCategory('');
        setType('');
        setUseCustomType(false);
        setDateYear('');
        setDateMonth('');
        setDateDay('');
        setDescription('');
        setRemind(false);
        setRelatedContacts([]);
      }
      setError('');
      setSearchInput('');
    }
  }, [open, initial]);

  const currentDate: PartialDate | undefined = (() => {
    const y = dateYear ? parseInt(dateYear, 10) : undefined;
    const m = dateMonth ? parseInt(dateMonth, 10) : undefined;
    const d = dateDay ? parseInt(dateDay, 10) : undefined;
    if (y == null && m == null && d == null) return undefined;
    return { year: y, month: m, day: d };
  })();

  const canRemind = partialDateHasMonthDay(currentDate);
  const isYearOnly = partialDateIsYearOnly(currentDate);

  const handleCategoryChange = (value: LifeEventCategory | typeof UNCATEGORIZED) => {
    setCategory(value);
    setType('');
    setUseCustomType(false);
    setError('');
  };

  const handleTypeSelectChange = (value: string) => {
    if (value === CUSTOM_TYPE) {
      setUseCustomType(true);
      setType('');
    } else {
      setUseCustomType(false);
      setType(value);
    }
    setError('');
  };

  const handleSave = async () => {
    if (!category) {
      setError(t('lifeEvent.validation.categoryRequired'));
      return;
    }
    if (!type) {
      setError(t('lifeEvent.validation.typeRequired'));
      return;
    }

    setSaving(true);
    try {
      await onSave({
        type,
        category: category === UNCATEGORIZED ? undefined : category,
        date: currentDate,
        description: description || undefined,
        relatedEntityIds: relatedContacts.map((c) => c.uid),
        remind: canRemind ? remind : false,
      });
      handleClose();
    } catch {
      setError(t('lifeEvent.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{isEditing ? t('lifeEvent.editTitle') : t('lifeEvent.createTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            select
            label={t('lifeEvent.category')}
            value={category}
            onChange={(e) =>
              handleCategoryChange(e.target.value as LifeEventCategory | typeof UNCATEGORIZED)
            }
            fullWidth
            required
          >
            {LIFE_EVENT_CATEGORIES.map((cat) => (
              <MenuItem key={cat} value={cat}>
                {t(`lifeEvent.categories.${cat}`)}
              </MenuItem>
            ))}
            {/* Only offered when it's already the current (legacy/unrecognized)
                state — a brand-new event, or one already filed under a real
                category, must always get one of the five real categories;
                "Other / Uncategorized" is display/edit-graceful-degradation
                for existing data, not a choice a user can newly opt into. */}
            {category === UNCATEGORIZED && (
              <MenuItem value={UNCATEGORIZED}>{t('lifeEvent.categories.uncategorized')}</MenuItem>
            )}
          </TextField>

          {category === '' && (
            <TextField
              label={t('lifeEvent.type')}
              value=""
              placeholder={t('lifeEvent.selectCategoryFirst')}
              fullWidth
              disabled
            />
          )}

          {category === UNCATEGORIZED && (
            <TextField
              label={t('lifeEvent.type')}
              value={type}
              onChange={(e) => {
                setType(e.target.value);
                setError('');
              }}
              fullWidth
              required
            />
          )}

          {category !== '' && category !== UNCATEGORIZED && (
            <>
              <TextField
                select
                label={t('lifeEvent.type')}
                value={useCustomType ? CUSTOM_TYPE : type}
                onChange={(e) => handleTypeSelectChange(e.target.value)}
                fullWidth
                required
              >
                {LIFE_EVENT_TYPES_BY_CATEGORY[category].map((tkn) => (
                  <MenuItem key={tkn} value={tkn}>
                    {t(`lifeEvent.types.${tkn}`, tkn)}
                  </MenuItem>
                ))}
                <MenuItem value={CUSTOM_TYPE}>{t('lifeEvent.addCustomType')}</MenuItem>
              </TextField>
              {useCustomType && (
                <TextField
                  label={t('lifeEvent.customTypeLabel')}
                  value={type}
                  onChange={(e) => {
                    setType(e.target.value);
                    setError('');
                  }}
                  fullWidth
                  required
                  autoFocus
                />
              )}
            </>
          )}

          <Box
            sx={{
              display: 'flex',
              gap: 1,
            }}
          >
            <TextField
              label={t('lifeEvent.dateYear')}
              type="number"
              value={dateYear}
              onChange={(e) => setDateYear(e.target.value)}
              fullWidth
              slotProps={{ inputLabel: { shrink: true }, htmlInput: { min: 1900, max: 2100 } }}
            />
            <TextField
              label={t('lifeEvent.dateMonth')}
              type="number"
              value={dateMonth}
              onChange={(e) => setDateMonth(e.target.value)}
              fullWidth
              slotProps={{ inputLabel: { shrink: true }, htmlInput: { min: 1, max: 12 } }}
            />
            <TextField
              label={t('lifeEvent.dateDay')}
              type="number"
              value={dateDay}
              onChange={(e) => setDateDay(e.target.value)}
              fullWidth
              slotProps={{ inputLabel: { shrink: true }, htmlInput: { min: 1, max: 31 } }}
            />
          </Box>

          <TextField
            label={t('lifeEvent.description')}
            multiline
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            fullWidth
          />

          <Autocomplete
            multiple
            options={contacts}
            getOptionLabel={contactLabel}
            value={relatedContacts}
            onChange={(_, value) => setRelatedContacts(value)}
            onInputChange={(_, value, reason) => {
              if (reason === 'input') setSearchInput(value);
            }}
            loading={contactsLoading}
            filterOptions={(x) => x}
            renderInput={(params) => (
              <TextField
                {...params}
                label={t('lifeEvent.relatedEntities')}
                placeholder={t('lifeEvent.searchContacts')}
                slotProps={{
                  ...params.slotProps,

                  input: {
                    ...params.slotProps.input,
                    endAdornment: (
                      <>
                        {contactsLoading ? <CircularProgress color="inherit" size={20} /> : null}
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
                  label={contactLabel(contact)}
                  {...getItemProps({ index })}
                  size="small"
                  key={contact.uid}
                />
              ))
            }
            isOptionEqualToValue={(option, value) => option.uid === value.uid}
            noOptionsText={
              searchInput ? t('lifeEvent.noContactsFound') : t('lifeEvent.typeToSearch')
            }
          />

          <Tooltip
            title={
              isYearOnly
                ? t('lifeEvent.remindDisabledYearOnly')
                : !currentDate
                  ? t('lifeEvent.remindDisabledNoDate')
                  : ''
            }
          >
            <FormControlLabel
              control={
                <Checkbox
                  checked={remind}
                  onChange={(e) => setRemind(e.target.checked)}
                  disabled={!canRemind}
                />
              }
              label={t('lifeEvent.remind')}
            />
          </Tooltip>

          {error && (
            <Typography color="error" variant="body2">
              {error}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('lifeEvent.cancel')}
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {t('lifeEvent.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
