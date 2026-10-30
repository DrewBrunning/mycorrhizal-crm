import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ContactFieldVisibility, FieldStateInput } from '../api/contactFieldVisibility';
import { CONTACT_FIELD_GROUPS, CONTACT_FIELDS, type ContactFieldKey } from '../contactFields';

// "Customize fields" for ONE contact (issue #1997): per-field show / hide /
// default and the hide-empty mode. Purely a display preference -- the dialog
// says so, and nothing here touches the contact's data or what syncs.

interface Props {
  open: boolean;
  onClose: () => void;
  visibility: ContactFieldVisibility | null;
  onSave: (input: {
    overrides: Partial<Record<ContactFieldKey, FieldStateInput>>;
    hide_empty: FieldStateInput;
  }) => Promise<void>;
  onReset: () => Promise<void>;
}

type Draft = Partial<Record<ContactFieldKey, FieldStateInput>>;

function draftFrom(v: ContactFieldVisibility | null): Draft {
  const out: Draft = {};
  for (const [key, state] of Object.entries(v?.overrides ?? {}))
    out[key as ContactFieldKey] = state;
  return out;
}

export default function ContactFieldVisibilityDialog({
  open,
  onClose,
  visibility,
  onSave,
  onReset,
}: Props) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>({});
  const [hideEmpty, setHideEmpty] = useState<FieldStateInput>('inherit');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // Re-seed from the server state every time the dialog opens.
  useEffect(() => {
    if (!open) return;
    setDraft(draftFrom(visibility));
    setHideEmpty(visibility?.hide_empty ?? 'inherit');
    setError('');
  }, [open, visibility]);

  const stateWord = (shown: boolean) =>
    shown
      ? t('contactDetail.fieldVisibility.dialog.stateShown')
      : t('contactDetail.fieldVisibility.dialog.stateHidden');

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await action();
      onClose();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t('contactDetail.fieldVisibility.dialog.saveError'),
      );
    } finally {
      setBusy(false);
    }
  };

  const handleSave = () => {
    // inherit entries are sent too: the PUT is a full replace and the server
    // stores only show/hide.
    void run(() => onSave({ overrides: draft, hide_empty: hideEmpty }));
  };

  const accountHideEmpty = visibility?.defaults.hide_empty ?? false;

  return (
    <Dialog open={open} onClose={busy ? undefined : onClose} fullWidth maxWidth="sm" scroll="paper">
      <DialogTitle>{t('contactDetail.fieldVisibility.dialog.title')}</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            {t('contactDetail.fieldVisibility.dialog.description')}
          </Typography>
          {error && <Alert severity="error">{error}</Alert>}

          <Box>
            <Typography variant="overline" color="text.secondary" component="h3">
              {t('contactDetail.fieldVisibility.dialog.emptyFields')}
            </Typography>
            <ToggleButtonGroup
              exclusive
              fullWidth
              size="small"
              orientation="vertical"
              value={hideEmpty}
              aria-label={t('contactDetail.fieldVisibility.dialog.emptyFields')}
              onChange={(_, v: FieldStateInput | null) => {
                if (v) setHideEmpty(v);
              }}
            >
              <ToggleButton value="inherit">
                {t('contactDetail.fieldVisibility.dialog.hideEmptyInherit', {
                  state: accountHideEmpty
                    ? t('contactDetail.fieldVisibility.dialog.stateOn')
                    : t('contactDetail.fieldVisibility.dialog.stateOff'),
                })}
              </ToggleButton>
              <ToggleButton value="hide">
                {t('contactDetail.fieldVisibility.dialog.hideEmptyHide')}
              </ToggleButton>
              <ToggleButton value="show">
                {t('contactDetail.fieldVisibility.dialog.hideEmptyShow')}
              </ToggleButton>
            </ToggleButtonGroup>
          </Box>

          {CONTACT_FIELD_GROUPS.map((group) => {
            const fields = CONTACT_FIELDS.filter((f) => f.group === group);
            if (fields.length === 0) return null;
            return (
              <Box key={group}>
                <Typography variant="overline" color="text.secondary" component="h3">
                  {t(`settings.contactFields.groups.${group}`)}
                </Typography>
                <Stack spacing={1}>
                  {fields.map((f) => {
                    const label = t(f.labelKey);
                    return (
                      <Box
                        key={f.key}
                        sx={{
                          display: 'flex',
                          flexDirection: { xs: 'column', sm: 'row' },
                          alignItems: { xs: 'stretch', sm: 'center' },
                          justifyContent: 'space-between',
                          gap: 1,
                        }}
                      >
                        <Typography variant="body2" id={`field-vis-${f.key}`}>
                          {label}
                        </Typography>
                        <ToggleButtonGroup
                          exclusive
                          size="small"
                          value={draft[f.key] ?? 'inherit'}
                          aria-labelledby={`field-vis-${f.key}`}
                          onChange={(_, v: FieldStateInput | null) => {
                            if (v) setDraft((d) => ({ ...d, [f.key]: v }));
                          }}
                        >
                          <ToggleButton value="inherit">
                            {t('contactDetail.fieldVisibility.dialog.fieldInherit', {
                              state: stateWord(visibility?.defaults.fields[f.key] ?? false),
                            })}
                          </ToggleButton>
                          <ToggleButton value="show">
                            {t('contactDetail.fieldVisibility.dialog.fieldShow')}
                          </ToggleButton>
                          <ToggleButton value="hide">
                            {t('contactDetail.fieldVisibility.dialog.fieldHide')}
                          </ToggleButton>
                        </ToggleButtonGroup>
                      </Box>
                    );
                  })}
                </Stack>
              </Box>
            );
          })}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ justifyContent: 'space-between' }}>
        <Button disabled={busy} onClick={() => void run(onReset)}>
          {t('contactDetail.fieldVisibility.dialog.reset')}
        </Button>
        <Box>
          <Button disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="contained" disabled={busy} onClick={handleSave}>
            {t('common.save')}
          </Button>
        </Box>
      </DialogActions>
    </Dialog>
  );
}
