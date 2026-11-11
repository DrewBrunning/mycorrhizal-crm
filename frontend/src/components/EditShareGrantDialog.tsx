import {
  Alert,
  Box,
  Button,
  Checkbox,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormGroup,
  FormLabel,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isPreconditionFailed } from '../api/client';
import {
  type ShareGrantOwnerView,
  type ShareGrantPatchInput,
  updateShareGrant,
} from '../api/shareGrants';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';
import DateField from './DateField';

interface EditShareGrantDialogProps {
  grant: ShareGrantOwnerView | null;
  onClose: () => void;
  onSaved: (grant: ShareGrantOwnerView) => void;
  // Called after a 412 so the page can reload the list.
  onStale: () => void;
}

const toDateInput = (iso: string | null) => (iso ? iso.slice(0, 10) : '');

function endOfDayISO(date: string): string {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d, 23, 59, 59).toISOString();
}

// Narrow-only edit (ADR 0042 decision 12): the expiry may change and sections may
// be removed. Only the grant's CURRENT sections are listed, so widening is not
// offered (the API refuses it with 409 anyway). Only what changed is sent.
export default function EditShareGrantDialog(props: EditShareGrantDialogProps) {
  const { grant } = props;
  // Remount per grant so the initial state is that grant's.
  return grant ? <Inner key={grant.id} {...props} grant={grant} /> : null;
}

function Inner({
  grant,
  onClose,
  onSaved,
  onStale,
}: EditShareGrantDialogProps & { grant: ShareGrantOwnerView }) {
  const { t } = useTranslation();
  const [kept, setKept] = useState<Set<string>>(new Set(grant.sections));
  const [expiry, setExpiry] = useState(toDateInput(grant.expires_at));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const hasSections = grant.entity_type === 'contact';

  const sectionsChanged = hasSections && kept.size !== grant.sections.length;
  const expiryChanged = expiry !== toDateInput(grant.expires_at);
  const valid = !hasSections || kept.size > 0;
  const canSave = !saving && valid && (sectionsChanged || expiryChanged);

  const save = async () => {
    const patch: ShareGrantPatchInput = {};
    if (sectionsChanged) patch.sections = grant.sections.filter((s) => kept.has(s));
    if (expiryChanged) patch.expires_at = expiry ? endOfDayISO(expiry) : null;
    setSaving(true);
    setError('');
    try {
      onSaved(await updateShareGrant(grant.id, patch, grant.revision));
    } catch (err) {
      if (isPreconditionFailed(err)) {
        setError(t('standingShares.byMe.conflict'));
        onStale();
      } else {
        setError(getErrorMessage(err));
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{t('standingShares.byMe.editTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('standingShares.byMe.editNarrowOnly')}
          </Typography>
          {hasSections && (
            <FormControl component="fieldset" variant="standard">
              <FormLabel component="legend">{t('standingShares.form.sections')}</FormLabel>
              <FormGroup>
                {grant.sections.map((s) => (
                  <FormControlLabel
                    key={s}
                    control={
                      <Checkbox
                        checked={kept.has(s)}
                        onChange={(e) =>
                          setKept((prev) => {
                            const next = new Set(prev);
                            if (e.target.checked) next.add(s);
                            else next.delete(s);
                            return next;
                          })
                        }
                      />
                    }
                    label={t(`settings.exportFieldPicker.sections.${s}`, { defaultValue: s })}
                  />
                ))}
              </FormGroup>
            </FormControl>
          )}
          <DateField
            label={t('standingShares.byMe.expiresField')}
            value={expiry}
            onChange={(v) => setExpiry(v)}
          />
          {error && <Alert severity="error">{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button variant="contained" onClick={() => void save()} disabled={!canSave}>
          {t('standingShares.byMe.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
