import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  DialogActions,
  DialogContent,
  FormControl,
  FormControlLabel,
  FormGroup,
  FormHelperText,
  FormLabel,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getUserDirectory, type UserDirectoryEntry } from '../api/contactShares';
import { EXPORT_FIELD_SECTIONS } from '../api/export';
import {
  createShareGrant,
  getShareJSContactPreview,
  type JSContactCard,
  type ShareGrantEntityType,
  type ShareGrantOwnerView,
} from '../api/shareGrants';
import { getErrorMessage } from '../utils/errorHandler';
import SharedContactCard from './SharedContactCard';

interface StandingShareFormProps {
  entityType: ShareGrantEntityType;
  // Contact: vcard_uid. Note/activity: UUID. Circle: its id.
  entityId: string;
  onCreated: (grant: ShareGrantOwnerView) => void;
  onCancel: () => void;
  // Called on every user edit so a hosting dialog can arm its discard guard.
  onDirty?: () => void;
}

// An expiry date (YYYY-MM-DD) means "through the end of that local day".
function endOfDayISO(date: string): string {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d, 23, 59, 59).toISOString();
}

function newIdempotencyKey(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

// StandingShareForm is the "standing" half of the Share dialog (ADR 0042
// decision 12; issue #2292): a revocable, read-only, live grant to one other
// user. Deliberate choices:
//  - a contact grant starts with NO section selected (no "all" default) and
//    cannot be created until one is chosen;
//  - there is no include-sensitive control: private and secret items are never
//    shared, so the reveal flow of the one-time dialog does not apply;
//  - the contact preview is the single-contact JSContact export for the chosen
//    sections, built by the same projection path the grant read uses, so a
//    field hidden in the owner's own UI but inside a granted section is shown;
//  - the copy states that revocation is not recall.
export default function StandingShareForm({
  entityType,
  entityId,
  onCreated,
  onCancel,
  onDirty,
}: StandingShareFormProps) {
  const { t } = useTranslation();
  const [recipients, setRecipients] = useState<UserDirectoryEntry[]>([]);
  const [recipient, setRecipient] = useState<UserDirectoryEntry | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [expiry, setExpiry] = useState('');
  const [sharing, setSharing] = useState(false);
  const [error, setError] = useState('');
  const [preview, setPreview] = useState<{
    status: 'idle' | 'loading' | 'error' | 'ready';
    card: JSContactCard | null;
  }>({ status: 'idle', card: null });
  // One key per form instance: a retry after a network failure replays the same
  // create instead of risking a duplicate (the backend honours Idempotency-Key).
  const idempotencyKey = useRef(newIdempotencyKey());
  const isContact = entityType === 'contact';

  useEffect(() => {
    let cancelled = false;
    getUserDirectory()
      .then((users) => {
        if (!cancelled) setRecipients(users);
      })
      .catch((err) => {
        if (!cancelled) setError(getErrorMessage(err));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const sections = useMemo(() => [...selected].sort(), [selected]);
  useEffect(() => {
    if (!isContact) return;
    if (sections.length === 0) {
      setPreview({ status: 'idle', card: null });
      return;
    }
    let cancelled = false; // a slower earlier response must not overwrite a newer one
    setPreview({ status: 'loading', card: null });
    getShareJSContactPreview(entityId, sections)
      .then((card) => {
        if (!cancelled) setPreview({ status: 'ready', card });
      })
      .catch(() => {
        if (!cancelled) setPreview({ status: 'error', card: null });
      });
    return () => {
      cancelled = true;
    };
  }, [isContact, entityId, sections]);

  const toggle = (token: string, checked: boolean) => {
    onDirty?.();
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) next.add(token);
      else next.delete(token);
      return next;
    });
  };

  const canSubmit = !sharing && recipient !== null && (!isContact || selected.size > 0);

  const handleShare = async () => {
    if (!recipient) return;
    setSharing(true);
    setError('');
    try {
      const grant = await createShareGrant(
        {
          to_user_id: recipient.id,
          entity_type: entityType,
          entity_id: entityId,
          ...(isContact ? { sections: [...selected] } : {}),
          ...(expiry ? { expires_at: endOfDayISO(expiry) } : {}),
        },
        idempotencyKey.current,
      );
      onCreated(grant);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSharing(false);
    }
  };

  const previewBody = (() => {
    if (preview.status === 'loading') {
      return <Typography variant="body2">{t('standingShares.form.previewLoading')}</Typography>;
    }
    if (preview.status === 'error') {
      return (
        <Typography variant="body2" color="error">
          {t('standingShares.form.previewError')}
        </Typography>
      );
    }
    if (preview.status === 'ready' && preview.card) {
      return <SharedContactCard card={preview.card} />;
    }
    return null;
  })();

  return (
    <>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('standingShares.form.description')}
          </Typography>
          <Alert severity="info" sx={{ py: 0 }}>
            {t('standingShares.form.noRecallNotice')}
          </Alert>
          {isContact && (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('standingShares.form.liveNotice')}
            </Typography>
          )}

          <Autocomplete
            options={recipients}
            value={recipient}
            onChange={(_e, value) => {
              onDirty?.();
              setRecipient(value);
            }}
            getOptionLabel={(u) => u.username}
            isOptionEqualToValue={(a, b) => a.id === b.id}
            noOptionsText={t('standingShares.form.noRecipients')}
            renderInput={(params) => (
              <TextField {...params} label={t('standingShares.form.recipient')} />
            )}
          />

          {isContact ? (
            <FormControl component="fieldset" variant="standard">
              <FormLabel component="legend">{t('standingShares.form.sections')}</FormLabel>
              <FormGroup>
                {EXPORT_FIELD_SECTIONS.map((section) => (
                  <FormControlLabel
                    key={section.token}
                    control={
                      <Checkbox
                        checked={selected.has(section.token)}
                        onChange={(e) => toggle(section.token, e.target.checked)}
                      />
                    }
                    label={t(`settings.exportFieldPicker.sections.${section.token}`)}
                  />
                ))}
              </FormGroup>
              <FormHelperText>{t('standingShares.form.sectionsHint')}</FormHelperText>
            </FormControl>
          ) : (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t(`standingShares.form.entityNotice.${entityType}`)}
            </Typography>
          )}

          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('standingShares.form.privateNotice')}
          </Typography>

          <TextField
            type="date"
            label={t('standingShares.form.expires')}
            value={expiry}
            onChange={(e) => {
              onDirty?.();
              setExpiry(e.target.value);
            }}
            slotProps={{ inputLabel: { shrink: true } }}
          />

          {isContact && selected.size > 0 && (
            <Box
              component="section"
              aria-labelledby="standing-share-preview-title"
              sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}
            >
              <Typography id="standing-share-preview-title" variant="subtitle2" component="h3">
                {t('standingShares.form.previewTitle')}
              </Typography>
              <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block' }}>
                {t('standingShares.form.previewHint')}
              </Typography>
              <Box sx={{ mt: 1 }}>{previewBody}</Box>
            </Box>
          )}

          {error && (
            <Alert severity="error" sx={{ py: 0 }}>
              {error}
            </Alert>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} disabled={sharing}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleShare()} variant="contained" disabled={!canSubmit}>
          {sharing ? t('standingShares.form.submitting') : t('standingShares.form.submit')}
        </Button>
      </DialogActions>
    </>
  );
}
