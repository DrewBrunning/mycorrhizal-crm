import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import KeyIcon from '@mui/icons-material/Key';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  List,
  ListItem,
  ListItemText,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Passkey, proveWithOtherPasskey } from '../api/webauthn';
import { useSnackbar } from '../context/SnackbarContext';
import { useWebAuthn } from '../hooks/useWebAuthn';
import { isCeremonyCancelled, isWebAuthnSupported } from '../webauthnCeremony';
import AppDialog from './AppDialog';

// PasskeySettings is the settings-page card for passkeys (issue #594, the
// frontend half of #593): enroll via the browser's WebAuthn API, list, and
// remove. Removal needs a live second-factor proof (ASVS 3.7.1), so it reuses
// TwoFactorSettings' code-prompt dialog shape — with an extra "use another
// passkey" route for accounts that have no authenticator app.
export default function PasskeySettings() {
  const { t, i18n } = useTranslation();
  const { showSuccess } = useSnackbar();
  const { passkeys, loading, register, remove } = useWebAuthn();
  const supported = isWebAuthnSupported();

  const [name, setName] = useState('');
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState('');
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [copied, setCopied] = useState(false);

  // Removal proof dialog: `removing` is the passkey being removed.
  const [removing, setRemoving] = useState<Passkey | null>(null);
  const [proofCode, setProofCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [removeError, setRemoveError] = useState('');

  const formatDate = (iso?: string | null) =>
    iso ? new Date(iso).toLocaleDateString(i18n.language) : '';

  const handleAdd = async () => {
    setAdding(true);
    setError('');
    try {
      const result = await register(name);
      setName('');
      if (result.recovery_codes.length > 0) {
        setRecoveryCodes(result.recovery_codes);
        setCopied(false);
      }
      showSuccess(t('settings.passkeys.addSuccess'));
    } catch (err) {
      setError(
        isCeremonyCancelled(err)
          ? t('settings.passkeys.cancelled')
          : err instanceof Error && err.message
            ? err.message
            : t('settings.passkeys.addError'),
      );
    } finally {
      setAdding(false);
    }
  };

  const openRemove = (passkey: Passkey) => {
    setRemoving(passkey);
    setProofCode('');
    setRemoveError('');
  };

  const finishRemove = async (proof: Parameters<typeof remove>[1]) => {
    if (!removing) return;
    setBusy(true);
    setRemoveError('');
    try {
      await remove(removing.id, proof);
      setRemoving(null);
      setProofCode('');
      showSuccess(t('settings.passkeys.removeSuccess'));
    } catch (err) {
      setRemoveError(
        isCeremonyCancelled(err)
          ? t('settings.passkeys.cancelled')
          : err instanceof Error && err.message
            ? err.message
            : t('settings.passkeys.invalidProof'),
      );
    } finally {
      setBusy(false);
    }
  };

  const submitRemove = (e: React.FormEvent) => {
    e.preventDefault();
    void finishRemove({ code: proofCode.trim() });
  };

  const removeWithPasskey = async () => {
    setBusy(true);
    setRemoveError('');
    try {
      await finishRemove(await proveWithOtherPasskey());
    } catch (err) {
      setRemoveError(
        isCeremonyCancelled(err)
          ? t('settings.passkeys.cancelled')
          : err instanceof Error && err.message
            ? err.message
            : t('settings.passkeys.invalidProof'),
      );
      setBusy(false);
    }
  };

  const handleCopyCodes = () => {
    if (recoveryCodes) {
      void navigator.clipboard.writeText(recoveryCodes.join('\n'));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const closeRecoveryDialog = () => {
    setRecoveryCodes(null);
    setCopied(false);
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <KeyIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('settings.passkeys.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {loading ? (
          <CircularProgress size={24} />
        ) : (
          <Stack spacing={1}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('settings.passkeys.description')}
            </Typography>

            {passkeys.length === 0 ? (
              <Typography variant="body2">{t('settings.passkeys.empty')}</Typography>
            ) : (
              <List dense disablePadding aria-label={t('settings.passkeys.listLabel')}>
                {passkeys.map((p) => (
                  <ListItem
                    key={p.id}
                    disableGutters
                    secondaryAction={
                      <Button
                        size="small"
                        color="error"
                        onClick={() => openRemove(p)}
                        aria-label={t('settings.passkeys.removeAria', { name: p.name })}
                      >
                        {t('settings.passkeys.removeButton')}
                      </Button>
                    }
                  >
                    <ListItemText
                      primary={p.name}
                      secondary={
                        p.last_used_at
                          ? t('settings.passkeys.createdLastUsed', {
                              created: formatDate(p.created_at),
                              lastUsed: formatDate(p.last_used_at),
                            })
                          : t('settings.passkeys.createdNeverUsed', {
                              created: formatDate(p.created_at),
                            })
                      }
                    />
                  </ListItem>
                ))}
              </List>
            )}

            {!supported ? (
              <Alert severity="info" sx={{ py: 0 }}>
                {t('settings.passkeys.unsupported')}
              </Alert>
            ) : (
              <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                <TextField
                  label={t('settings.passkeys.nameLabel')}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  size="small"
                  disabled={adding}
                  slotProps={{ htmlInput: { maxLength: 100 } }}
                  helperText={t('settings.passkeys.nameHelp')}
                />
                <Button
                  variant="contained"
                  size="small"
                  sx={{ mt: 0.5 }}
                  onClick={() => void handleAdd()}
                  disabled={adding}
                >
                  {adding ? t('settings.passkeys.adding') : t('settings.passkeys.addButton')}
                </Button>
              </Stack>
            )}
            {error && (
              <Alert severity="error" sx={{ py: 0 }}>
                {error}
              </Alert>
            )}
          </Stack>
        )}
      </CardContent>

      {/* Live second-factor proof for removal */}
      <AppDialog
        open={removing !== null}
        onClose={() => !busy && setRemoving(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>{t('settings.passkeys.remove.title')}</DialogTitle>
        <DialogContent>
          <form onSubmit={submitRemove}>
            <Stack spacing={1.5}>
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('settings.passkeys.remove.description', { name: removing?.name ?? '' })}
              </Typography>
              <TextField
                label={t('settings.passkeys.remove.codeLabel')}
                type="text"
                value={proofCode}
                onChange={(e) => setProofCode(e.target.value)}
                required
                fullWidth
                size="small"
                autoFocus
              />
              {removeError && (
                <Alert severity="error" sx={{ py: 0 }}>
                  {removeError}
                </Alert>
              )}
              <Button
                type="submit"
                variant="contained"
                color="error"
                disabled={busy || proofCode.trim().length === 0}
              >
                {busy
                  ? t('settings.passkeys.remove.submitting')
                  : t('settings.passkeys.remove.confirm')}
              </Button>
              {passkeys.length > 1 && supported && (
                <Button variant="outlined" onClick={() => void removeWithPasskey()} disabled={busy}>
                  {t('settings.passkeys.remove.useAnotherPasskey')}
                </Button>
              )}
            </Stack>
          </form>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRemoving(null)} disabled={busy}>
            {t('common.cancel')}
          </Button>
        </DialogActions>
      </AppDialog>

      {/* Recovery codes — minted with the account's first second factor,
          shown exactly once. Reuses the TOTP wizard's copy. */}
      <Dialog open={recoveryCodes !== null} onClose={closeRecoveryDialog} maxWidth="sm" fullWidth>
        <DialogTitle>{t('settings.twoFactor.recovery.title')}</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            {t('settings.twoFactor.recovery.warning')}
          </Alert>
          <Box
            sx={{
              fontFamily: 'monospace',
              fontSize: '0.9rem',
              border: 1,
              borderColor: 'divider',
              borderRadius: 1,
              p: 1.5,
              display: 'grid',
              gridTemplateColumns: '1fr 1fr',
              gap: 0.5,
            }}
          >
            {recoveryCodes?.map((code) => (
              <Typography key={code} variant="body2" sx={{ fontFamily: 'monospace' }}>
                {code}
              </Typography>
            ))}
          </Box>
          <Box sx={{ mt: 1 }}>
            <Button size="small" startIcon={<ContentCopyIcon />} onClick={handleCopyCodes}>
              {copied
                ? t('settings.twoFactor.recovery.copied')
                : t('settings.twoFactor.recovery.copy')}
            </Button>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button variant="contained" onClick={closeRecoveryDialog}>
            {t('settings.twoFactor.recovery.done')}
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
}
