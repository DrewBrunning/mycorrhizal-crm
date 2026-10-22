import {
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { CardPersonalInfo, PersonalInfoMeta } from '../api/contacts';
import { PERSONAL_INFO_KIND_OPTIONS, PERSONAL_INFO_LEVEL_OPTIONS } from '../contactFields';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import AppDialog from './AppDialog';

export interface PersonalInfoFormData {
  kind: string;
  value: string;
  level?: string;
  label?: string;
  notes?: string;
  sensitivity: NonNullable<PersonalInfoMeta['sensitivity']>;
  favorite: boolean;
}

interface PersonalInfoDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: PersonalInfoFormData) => Promise<void>;
  entry?: CardPersonalInfo | null;
  meta?: PersonalInfoMeta;
}

// Add/edit one Card.PersonalInfo entry (issue #1729, #1738): kind/value/level/
// label live on the Card; sensitivity/favorite/notes live in
// crm.personalInfoMeta keyed by the entry's stable ID. Hobbies are created
// here and only here -- the deprecated Preference hobby category is no longer
// offered by the add-preference dialog.
export default function PersonalInfoDialog({
  open,
  onClose,
  onSave,
  entry,
  meta,
}: PersonalInfoDialogProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<string>(PERSONAL_INFO_KIND_OPTIONS[0]);
  const [value, setValue] = useState('');
  const [level, setLevel] = useState('');
  const [label, setLabel] = useState('');
  const [notes, setNotes] = useState('');
  const [sensitivity, setSensitivity] =
    useState<NonNullable<PersonalInfoMeta['sensitivity']>>('normal');
  const [favorite, setFavorite] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (open) {
      setKind(entry?.kind || PERSONAL_INFO_KIND_OPTIONS[0]);
      setValue(entry?.value || '');
      setLevel(entry?.level || '');
      setLabel(entry?.label || '');
      setNotes(meta?.notes || '');
      setSensitivity(meta?.sensitivity || 'normal');
      setFavorite(!!meta?.favorite);
      setError('');
    }
  }, [open, entry, meta]);

  const handleSave = async () => {
    const trimmed = value.trim();
    if (!trimmed) {
      setError(t('contactDetail.skills.validation.valueRequired'));
      return;
    }
    setSaving(true);
    try {
      await onSave({
        kind,
        value: trimmed,
        level: level || undefined,
        label: label.trim() || undefined,
        notes: notes.trim() || undefined,
        sensitivity,
        favorite,
      });
      onClose();
    } catch {
      setError(t('contactDetail.skills.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {entry ? t('contactDetail.skills.editTitle') : t('contactDetail.skills.createTitle')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            select
            label={t('contacts.personalInfo.kind')}
            value={kind}
            onChange={(e) => {
              markDirty();
              setKind(e.target.value);
              setError('');
            }}
            fullWidth
            required
          >
            {PERSONAL_INFO_KIND_OPTIONS.map((opt) => (
              <MenuItem key={opt} value={opt}>
                {t(`contacts.personalInfo.kindOptions.${opt}`)}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            label={t('contacts.personalInfo.value')}
            value={value}
            onChange={(e) => {
              markDirty();
              setValue(e.target.value);
              setError('');
            }}
            fullWidth
            required
          />
          <TextField
            select
            label={t('contacts.personalInfo.level')}
            value={level}
            onChange={(e) => {
              markDirty();
              setLevel(e.target.value);
            }}
            fullWidth
          >
            <MenuItem value="">{t('contacts.personalInfo.levelNone')}</MenuItem>
            {PERSONAL_INFO_LEVEL_OPTIONS.map((opt) => (
              <MenuItem key={opt} value={opt}>
                {t(`contacts.personalInfo.levelOptions.${opt}`)}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            label={t('contacts.personalInfo.label')}
            value={label}
            onChange={(e) => {
              markDirty();
              setLabel(e.target.value);
            }}
            fullWidth
          />
          <TextField
            label={t('preference.notes')}
            value={notes}
            onChange={(e) => {
              markDirty();
              setNotes(e.target.value);
            }}
            helperText={t('preference.notesHint')}
            fullWidth
            multiline
            minRows={2}
          />
          <TextField
            select
            label={t('preference.sensitivity')}
            value={sensitivity}
            onChange={(e) => {
              markDirty();
              setSensitivity(e.target.value as NonNullable<PersonalInfoMeta['sensitivity']>);
            }}
            fullWidth
          >
            <MenuItem value="normal">{t('preference.sensitivities.normal')}</MenuItem>
            <MenuItem value="private">{t('preference.sensitivities.private')}</MenuItem>
            <MenuItem value="secret">{t('preference.sensitivities.secret')}</MenuItem>
          </TextField>
          <FormControlLabel
            control={
              <Switch
                checked={favorite}
                onChange={(e) => {
                  markDirty();
                  setFavorite(e.target.checked);
                }}
              />
            }
            label={t('contactDetail.skills.favorite')}
          />
          {error && (
            <Typography color="error" variant="body2">
              {error}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={requestClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleSave()} variant="contained" disabled={saving}>
          {t('common.save')}
        </Button>
      </DialogActions>
      {guardElement}
    </AppDialog>
  );
}
