import {
  Alert,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormLabel,
  MenuItem,
  Radio,
  RadioGroup,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  createContactShare,
  getUserDirectory,
  type UserDirectoryEntry,
} from '../api/contactShares';
import { EXPORT_FIELD_SECTIONS } from '../api/export';
import type { ShareGrantOwnerView } from '../api/shareGrants';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';
import FieldSectionPicker from './FieldSectionPicker';
import StandingShareForm from './StandingShareForm';

interface ShareContactDialogProps {
  open: boolean;
  onClose: () => void;
  vcardUID: string;
  onShared?: () => void;
  // Called after a standing share (ADR 0042) is created; the one-time flow
  // keeps using onShared.
  onStandingShared?: (grant: ShareGrantOwnerView) => void;
}

// ShareContactDialog is P1's "Share" dialog (P1) -- a one-time filtered copy sent to another user
// on the same instance. Reuses T9's FieldSectionPicker exactly (the same
// sensitivity foot-gun guard ExportFieldPickerDialog uses), swapping the
// format radio group for a recipient picker since JSContact is the only wire
// format a share round-trips through.
//
// Issue #555 (frozen-snapshot semantics, a decision recorded but never
// surfaced in the UI): the backend serializes the payload once, at
// share-creation time, and never re-derives it (models/contact_share.go's
// Payload doc comment) -- so a later edit to this contact, or a later
// re-classification of a field's sensitivity, does not reach an
// already-created share. frozenNotice below is the sender-facing statement
// of that fact, so "why doesn't my edit show up over there" isn't a support
// question.
export default function ShareContactDialog({
  open,
  onClose,
  vcardUID,
  onShared,
  onStandingShared,
}: ShareContactDialogProps) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'oneTime' | 'standing'>('oneTime');
  const [recipients, setRecipients] = useState<UserDirectoryEntry[]>([]);
  const [toUserID, setToUserID] = useState<number | ''>('');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sensitiveRevealed, setSensitiveRevealed] = useState(false);
  const [sharing, setSharing] = useState(false);
  const [error, setError] = useState('');
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  const defaultSelection = useMemo(
    () => new Set(EXPORT_FIELD_SECTIONS.filter((s) => !s.sensitive).map((s) => s.token)),
    [],
  );

  useEffect(() => {
    if (!open) return;
    setMode('oneTime');
    setSelected(new Set(defaultSelection));
    setSensitiveRevealed(false);
    setToUserID('');
    setError('');
    getUserDirectory()
      .then(setRecipients)
      .catch((err) => setError(getErrorMessage(err)));
  }, [open, defaultSelection]);

  const toggle = (token: string, checked: boolean) => {
    markDirty();
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(token);
      } else {
        next.delete(token);
      }
      return next;
    });
  };

  const handleShare = async () => {
    if (toUserID === '') return;
    const includeSensitive =
      sensitiveRevealed && EXPORT_FIELD_SECTIONS.some((s) => s.sensitive && selected.has(s.token));

    setSharing(true);
    setError('');
    try {
      await createContactShare({
        to_user_id: toUserID,
        vcard_uid: vcardUID,
        sections: [...selected],
        include_sensitive: includeSensitive,
      });
      onShared?.();
      onClose();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSharing(false);
    }
  };

  const modePicker = (
    <FormControl sx={{ px: 3, pt: 1 }}>
      <FormLabel id="share-mode-label">{t('standingShares.mode.label')}</FormLabel>
      <RadioGroup
        row
        aria-labelledby="share-mode-label"
        value={mode}
        onChange={(e) => {
          markDirty();
          setMode(e.target.value === 'standing' ? 'standing' : 'oneTime');
        }}
      >
        <FormControlLabel
          value="oneTime"
          control={<Radio />}
          label={t('standingShares.mode.oneTime')}
        />
        <FormControlLabel
          value="standing"
          control={<Radio />}
          label={t('standingShares.mode.standing')}
        />
      </RadioGroup>
    </FormControl>
  );

  if (mode === 'standing') {
    return (
      <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
        <DialogTitle>{t('contactShares.shareDialog.title')}</DialogTitle>
        {modePicker}
        <StandingShareForm
          entityType="contact"
          entityId={vcardUID}
          onDirty={markDirty}
          onCancel={requestClose}
          onCreated={(grant) => {
            onStandingShared?.(grant);
            onClose();
          }}
        />
        {guardElement}
      </AppDialog>
    );
  }

  return (
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('contactShares.shareDialog.title')}</DialogTitle>
      {modePicker}
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('contactShares.shareDialog.description')}
          </Typography>

          <Alert severity="info" sx={{ py: 0 }}>
            {t('contactShares.shareDialog.frozenNotice')}
          </Alert>

          <TextField
            select
            label={t('contactShares.shareDialog.recipient')}
            value={toUserID}
            onChange={(e) => {
              markDirty();
              setToUserID(e.target.value === '' ? '' : Number(e.target.value));
            }}
            fullWidth
          >
            <MenuItem value="">
              <em>{t('contactShares.shareDialog.selectRecipient')}</em>
            </MenuItem>
            {recipients.map((u) => (
              <MenuItem key={u.id} value={u.id}>
                {u.username}
              </MenuItem>
            ))}
          </TextField>

          <FieldSectionPicker
            selected={selected}
            onToggle={toggle}
            sensitiveRevealed={sensitiveRevealed}
            onReveal={() => {
              markDirty();
              setSensitiveRevealed(true);
            }}
          />

          {error && (
            <Alert severity="error" sx={{ py: 0 }}>
              {error}
            </Alert>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={requestClose} disabled={sharing}>
          {t('common.cancel')}
        </Button>
        <Button
          onClick={() => void handleShare()}
          variant="contained"
          disabled={sharing || toUserID === '' || selected.size === 0}
        >
          {sharing
            ? t('contactShares.shareDialog.sharing')
            : t('contactShares.shareDialog.shareButton')}
        </Button>
      </DialogActions>
      {guardElement}
    </AppDialog>
  );
}
