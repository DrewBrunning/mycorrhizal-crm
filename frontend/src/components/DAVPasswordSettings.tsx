import {
  Alert,
  Card,
  CardContent,
  CircularProgress,
  FormControlLabel,
  Switch,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getCurrentUser } from '../api/admin';
import { getTwoFactorStatus, updateDAVPasswordPolicy } from '../api/users';
import { listPasskeys } from '../api/webauthn';
import { useSnackbar } from '../context/SnackbarContext';

// Issue #2010: the account password is accepted on CardDAV/CalDAV only when
// the user opts in and has no second factor enrolled. This card makes that
// policy visible and adjustable.
export default function DAVPasswordSettings() {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const [allow, setAllow] = useState(false);
  const [hasSecondFactor, setHasSecondFactor] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const [user, status, passkeys] = await Promise.all([
        getCurrentUser(),
        getTwoFactorStatus(),
        listPasskeys(),
      ]);
      setAllow(!!user.allow_dav_password);
      setHasSecondFactor(!!status.enabled || passkeys.length > 0);
    } catch {
      setError(t('davPassword.loadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void load();
    const onSecondFactorChanged = () => void load();
    window.addEventListener('mycorrhizal:second-factor-changed', onSecondFactorChanged);
    return () =>
      window.removeEventListener('mycorrhizal:second-factor-changed', onSecondFactorChanged);
  }, [load]);

  const handleToggle = async (checked: boolean) => {
    setSaving(true);
    setError('');
    try {
      const next = await updateDAVPasswordPolicy(checked);
      setAllow(next);
      showSuccess(t('davPassword.success'));
    } catch (err) {
      setError(err instanceof Error ? err.message : t('davPassword.error'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500, mb: 1 }}>
          {t('davPassword.title')}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          {t('davPassword.description')}
        </Typography>
        {loading ? (
          <CircularProgress size={24} />
        ) : (
          <>
            {error && (
              <Alert severity="error" sx={{ mb: 1.5 }}>
                {error}
              </Alert>
            )}
            {hasSecondFactor && (
              <Alert severity="info" sx={{ mb: 1.5 }}>
                {t('davPassword.twoFactorNotice')}
              </Alert>
            )}
            <FormControlLabel
              control={
                <Switch
                  checked={allow && !hasSecondFactor}
                  disabled={saving || hasSecondFactor}
                  onChange={(event) => void handleToggle(event.target.checked)}
                />
              }
              label={t('davPassword.toggle')}
            />
          </>
        )}
      </CardContent>
    </Card>
  );
}
