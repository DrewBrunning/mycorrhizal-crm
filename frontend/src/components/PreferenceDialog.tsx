import {
  Autocomplete,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  ListSubheader,
  MenuItem,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  categorySupportsLevel,
  PREFERENCE_CATEGORY_CONFIG,
  PREFERENCE_DEFAULT_KEYS,
  PREFERENCE_LEVELS,
  type Preference,
  type PreferenceInput,
  type PreferenceLevel,
  type PreferenceSection,
  type PreferenceSensitivity,
} from '../api/preferences';
import AppDialog from './AppDialog';

export interface PreferenceFormData {
  category: string;
  key?: string;
  value: string;
  level?: PreferenceLevel;
  notes?: string;
  sensitivity: PreferenceSensitivity;
}

// Category options grouped by section (Food & Drink, Media, Activities &
// Hobbies, Jewelry & Style, Gift Preferences, Gift Avoid) for the dialog's
// ListSubheader-grouped select — same order PREFERENCE_CATEGORY_CONFIG
// declares them in.
const SECTION_ORDER: PreferenceSection[] = [
  'foodDrink',
  'media',
  'hobby',
  'jewelry',
  'giftPreferences',
  'giftAvoid',
];

interface PreferenceDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: PreferenceFormData) => Promise<void>;
  preference?: Preference | null;
  // Restricts the category dropdown (and the default selected category) to
  // these sections — e.g. the Gifts tab's dialog only offers
  // jewelry/giftPreferences/giftAvoid, so a shopping-focused "Add Preference"
  // can't accidentally create a food preference that then hides in the
  // Overview tab's list instead. Defaults to every section.
  sections?: PreferenceSection[];
}

export default function PreferenceDialog({
  open,
  onClose,
  onSave,
  preference,
  sections = SECTION_ORDER,
}: PreferenceDialogProps) {
  const { t } = useTranslation();
  const isEditing = !!preference;

  const availableCategories = PREFERENCE_CATEGORY_CONFIG.filter((c) =>
    sections.includes(c.section),
  );
  const defaultCategory = availableCategories[0]?.category ?? 'food';

  const [category, setCategory] = useState(defaultCategory);
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [level, setLevel] = useState<PreferenceLevel | ''>('');
  const [notes, setNotes] = useState('');
  const [sensitivity, setSensitivity] = useState<PreferenceSensitivity>('normal');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const showLevel = categorySupportsLevel(category);

  useEffect(() => {
    if (open) {
      setCategory(preference?.category || defaultCategory);
      setKey(preference?.key || '');
      setValue(preference?.value || '');
      setLevel(preference?.level || '');
      setNotes(preference?.notes || '');
      setSensitivity(preference?.sensitivity || 'normal');
      setError('');
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, preference]);

  const handleSave = async () => {
    if (!value.trim()) {
      setError(t('preference.validation.valueRequired'));
      return;
    }
    setSaving(true);
    try {
      const data: PreferenceFormData = {
        category,
        key: key.trim() || undefined,
        value: value.trim(),
        // A level only travels for hobby/skill categories (the backend rejects
        // it elsewhere); switching away clears it rather than stranding it.
        level: categorySupportsLevel(category) ? level || undefined : undefined,
        notes: notes.trim() || undefined,
        sensitivity,
      };
      await onSave(data);
      onClose();
    } catch {
      setError(t('preference.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {isEditing ? t('preference.editTitle') : t('preference.createTitle')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            select
            label={t('preference.category')}
            value={category}
            onChange={(e) => {
              const next = e.target.value;
              setCategory(next);
              if (!categorySupportsLevel(next)) setLevel('');
              setError('');
            }}
            fullWidth
            required
          >
            {sections.flatMap((section) => {
              const categories = availableCategories.filter((c) => c.section === section);
              if (categories.length === 0) return [];
              return [
                <ListSubheader key={`header-${section}`}>
                  {t(`preference.sections.${section}`)}
                </ListSubheader>,
                ...categories.map((c) => (
                  <MenuItem key={c.category} value={c.category}>
                    {t(`preference.categories.${c.category}`, c.category)}
                  </MenuItem>
                )),
              ];
            })}
          </TextField>
          <TextField
            label={t('preference.value')}
            value={value}
            onChange={(e) => {
              setValue(e.target.value);
              setError('');
            }}
            fullWidth
            required
          />
          <Autocomplete
            freeSolo
            options={PREFERENCE_DEFAULT_KEYS[category] ?? []}
            getOptionLabel={(opt) => t(`preference.keys.${opt}`, opt)}
            value={key || null}
            onChange={(_, v) => setKey(v || '')}
            onInputChange={(_, v) => setKey(v)}
            renderInput={(params) => (
              <TextField
                {...params}
                label={t('preference.key')}
                helperText={t('preference.keyHint')}
              />
            )}
          />
          {showLevel && (
            <TextField
              select
              label={t('preference.level')}
              value={level}
              onChange={(e) => setLevel(e.target.value as PreferenceLevel | '')}
              helperText={t('preference.levelHint')}
              fullWidth
            >
              <MenuItem value="">{t('preference.levels.none')}</MenuItem>
              {PREFERENCE_LEVELS.map((l) => (
                <MenuItem key={l} value={l}>
                  {t(`preference.levels.${l}`)}
                </MenuItem>
              ))}
            </TextField>
          )}
          <TextField
            label={t('preference.notes')}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            helperText={t('preference.notesHint')}
            fullWidth
            multiline
            minRows={2}
          />
          <TextField
            select
            label={t('preference.sensitivity')}
            value={sensitivity}
            onChange={(e) => setSensitivity(e.target.value as PreferenceSensitivity)}
            fullWidth
          >
            <MenuItem value="normal">{t('preference.sensitivities.normal')}</MenuItem>
            <MenuItem value="private">{t('preference.sensitivities.private')}</MenuItem>
            <MenuItem value="secret">{t('preference.sensitivities.secret')}</MenuItem>
          </TextField>
          {error && (
            <Typography color="error" variant="body2">
              {error}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('preference.cancel')}
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {t('preference.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}

// Rebuilds a PreferenceInput for the wire from a form payload + the target
// entity. source defaults to user, confidence to 1.0 — the same defaults the
// backend applies to a user-created preference.
export function toPreferenceInput(entityId: string, data: PreferenceFormData): PreferenceInput {
  return {
    entity_id: entityId,
    category: data.category,
    key: data.key,
    value: data.value,
    // Guard the category gate here too, so a form payload constructed by hand
    // (tests, other callers) can't send a level the backend will reject.
    level: categorySupportsLevel(data.category) ? data.level : undefined,
    notes: data.notes,
    source: 'user',
    confidence: 1.0,
    sensitivity: data.sensitivity,
  };
}
