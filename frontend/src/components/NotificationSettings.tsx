import DeleteIcon from '@mui/icons-material/Delete';
import NotificationsIcon from '@mui/icons-material/Notifications';
import NotificationsActiveIcon from '@mui/icons-material/NotificationsActive';
import PhoneIphoneIcon from '@mui/icons-material/PhoneIphone';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  FormControlLabel,
  IconButton,
  List,
  ListItem,
  ListItemText,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  createPushSubscription,
  type DeviceRegistration,
  deleteDeviceRegistration,
  deletePushSubscription,
  getDeviceRegistrations,
  getNotificationConfig,
  getPushSubscriptions,
  type NotificationChannel,
  type NotificationConfig,
  type PushSubscription,
  saveNotificationConfig,
  testNotificationChannel,
} from '../api/notifications';
import { useSnackbar } from '../context/SnackbarContext';
import { browserSupportsPush, subscribeBrowserPush } from '../pushSubscription';

// isPrivateAddressURL does a lightweight client-side check for the most
// common private/loopback targets so the Settings card can warn BEFORE the
// user saves a URL that WEBHOOK_BLOCK_PRIVATE_URLS would silently block.
// It is advisory only — the backend's dialer is the authoritative guard.
function isPrivateAddressURL(raw: string): boolean {
  const trimmed = raw.trim().toLowerCase();
  if (!/^https?:\/\//.test(trimmed)) return false;
  try {
    const hostname = new URL(trimmed).hostname;
    return (
      hostname === 'localhost' ||
      hostname === '127.0.0.1' ||
      hostname === '::1' ||
      hostname.endsWith('.localhost') ||
      /^127\./.test(hostname) ||
      /^10\./.test(hostname) ||
      /^192\.168\./.test(hostname) ||
      /^172\.(1[6-9]|2\d|3[01])\./.test(hostname) ||
      hostname === '0.0.0.0'
    );
  } catch {
    return false;
  }
}

export default function NotificationSettings() {
  const { t } = useTranslation();
  const { showSuccess, showError } = useSnackbar();

  const [config, setConfig] = useState<NotificationConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');

  const [ntfyUrl, setNtfyUrl] = useState('');
  const [ntfyTopic, setNtfyTopic] = useState('');
  const [gotifyUrl, setGotifyUrl] = useState('');
  const [gotifyToken, setGotifyToken] = useState('');
  const [notifyNtfy, setNotifyNtfy] = useState(false);
  const [notifyGotify, setNotifyGotify] = useState(false);
  const [notifyPush, setNotifyPush] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  const [testing, setTesting] = useState<NotificationChannel | null>(null);
  const [testResult, setTestResult] = useState<{
    channel: NotificationChannel;
    ok: boolean;
    error?: string;
  } | null>(null);

  const [subscriptions, setSubscriptions] = useState<PushSubscription[]>([]);
  const [subsLoading, setSubsLoading] = useState(false);
  const [enablingPush, setEnablingPush] = useState(false);

  const [devices, setDevices] = useState<DeviceRegistration[]>([]);
  const [devicesLoading, setDevicesLoading] = useState(false);

  const loadConfig = useCallback(async () => {
    try {
      setLoadError('');
      const data = await getNotificationConfig();
      setConfig(data);
      setNtfyUrl(data.ntfy_url || '');
      setNtfyTopic(data.ntfy_topic || '');
      setGotifyUrl(data.gotify_url || '');
      setGotifyToken('');
      setNotifyNtfy(data.notify_ntfy);
      setNotifyGotify(data.notify_gotify);
      setNotifyPush(data.notify_push);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : t('notifications.settings.loadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  const loadSubscriptions = useCallback(async () => {
    setSubsLoading(true);
    try {
      setSubscriptions(await getPushSubscriptions());
    } catch {
      // Device list failure is non-fatal; the rest of the card still works.
    } finally {
      setSubsLoading(false);
    }
  }, []);

  const loadDevices = useCallback(async () => {
    setDevicesLoading(true);
    try {
      setDevices(await getDeviceRegistrations());
    } catch {
      // Mobile device list failure is non-fatal; the rest of the card still works.
    } finally {
      setDevicesLoading(false);
    }
  }, []);

  useEffect(() => {
    loadConfig();
    loadSubscriptions();
    loadDevices();
  }, [loadConfig, loadSubscriptions, loadDevices]);

  const handleSave = async () => {
    setSaving(true);
    setSaveError('');
    setTestResult(null);
    try {
      const saved = await saveNotificationConfig({
        ntfy_url: ntfyUrl.trim(),
        ntfy_topic: ntfyTopic.trim(),
        gotify_url: gotifyUrl.trim(),
        gotify_token: gotifyToken.trim() || undefined,
        notify_ntfy: notifyNtfy,
        notify_gotify: notifyGotify,
        notify_push: notifyPush,
      });
      setConfig(saved);
      setNtfyUrl(saved.ntfy_url);
      setNtfyTopic(saved.ntfy_topic);
      setGotifyUrl(saved.gotify_url);
      setGotifyToken('');
      setNotifyNtfy(saved.notify_ntfy);
      setNotifyGotify(saved.notify_gotify);
      setNotifyPush(saved.notify_push);
      showSuccess(t('notifications.settings.saved'));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('notifications.settings.saveFailed');
      setSaveError(msg);
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async (channel: NotificationChannel) => {
    setTesting(channel);
    setTestResult(null);
    setSaveError('');
    try {
      // For the push-style channels, save first so the test uses what the user
      // is looking at, not the stale stored config.
      if (channel !== 'push') {
        await saveNotificationConfig({
          ntfy_url: ntfyUrl.trim(),
          ntfy_topic: ntfyTopic.trim(),
          gotify_url: gotifyUrl.trim(),
          gotify_token: gotifyToken.trim() || undefined,
          notify_ntfy: notifyNtfy,
          notify_gotify: notifyGotify,
          notify_push: notifyPush,
        });
      }
      const result = await testNotificationChannel(channel);
      setTestResult({ channel, ok: result.ok, error: result.error });
    } catch (err) {
      setTestResult({
        channel,
        ok: false,
        error: err instanceof Error ? err.message : t('notifications.settings.testFailed'),
      });
    } finally {
      setTesting(null);
    }
  };

  const handleEnablePush = async () => {
    if (!config) return;
    setEnablingPush(true);
    setSaveError('');
    try {
      const sub = await subscribeBrowserPush(config.vapid_public_key);
      await createPushSubscription(sub);
      await loadSubscriptions();
      showSuccess(t('notifications.settings.pushEnabled'));
    } catch (err) {
      const msg =
        err instanceof Error && err.message === 'push denied'
          ? t('notifications.settings.pushPermissionDenied')
          : err instanceof Error && err.message === 'push unsupported'
            ? t('notifications.settings.pushUnsupported')
            : err instanceof Error
              ? err.message
              : t('notifications.settings.pushEnableFailed');
      setSaveError(msg);
    } finally {
      setEnablingPush(false);
    }
  };

  const handleDeleteDevice = async (sub: PushSubscription) => {
    if (
      !window.confirm(
        t('notifications.settings.deleteDeviceConfirm', { label: sub.device_label || 'Browser' }),
      )
    ) {
      return;
    }
    try {
      await deletePushSubscription(sub.id);
      setSubscriptions((prev) => prev.filter((s) => s.id !== sub.id));
      showSuccess(t('notifications.settings.deviceDeleted'));
    } catch (err) {
      showError(
        err instanceof Error ? err.message : t('notifications.settings.deviceDeleteFailed'),
      );
    }
  };

  const handleDeleteMobileDevice = async (device: DeviceRegistration) => {
    if (
      !window.confirm(
        t('notifications.settings.deleteDeviceConfirm', {
          label: device.device_label || device.client,
        }),
      )
    ) {
      return;
    }
    try {
      await deleteDeviceRegistration(device.id);
      setDevices((prev) => prev.filter((d) => d.id !== device.id));
      showSuccess(t('notifications.settings.deviceDeleted'));
    } catch (err) {
      showError(
        err instanceof Error ? err.message : t('notifications.settings.deviceDeleteFailed'),
      );
    }
  };

  const ntfyPrivate = isPrivateAddressURL(ntfyUrl);
  const gotifyPrivate = isPrivateAddressURL(gotifyUrl);

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <NotificationsActiveIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('notifications.settings.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {loading ? (
          <CircularProgress size={24} />
        ) : (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              {t('notifications.settings.description')}
            </Typography>

            {loadError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {loadError}
              </Alert>
            )}
            {saveError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {saveError}
              </Alert>
            )}

            {/* ntfy */}
            <Box>
              <FormControlLabel
                control={
                  <Switch
                    checked={notifyNtfy}
                    onChange={(e) => setNotifyNtfy(e.target.checked)}
                    size="small"
                  />
                }
                label={t('notifications.settings.ntfy.enable')}
              />
              <Stack spacing={1.5} sx={{ mt: 1 }}>
                <TextField
                  label={t('notifications.settings.ntfy.url')}
                  value={ntfyUrl}
                  onChange={(e) => {
                    setNtfyUrl(e.target.value);
                    setSaveError('');
                    setTestResult(null);
                  }}
                  fullWidth
                  size="small"
                  placeholder="https://ntfy.sh"
                  helperText={t('notifications.settings.ntfy.urlHint')}
                />
                <TextField
                  label={t('notifications.settings.ntfy.topic')}
                  value={ntfyTopic}
                  onChange={(e) => {
                    setNtfyTopic(e.target.value);
                    setSaveError('');
                    setTestResult(null);
                  }}
                  fullWidth
                  size="small"
                  placeholder="my-reminders"
                />
                {ntfyPrivate && (
                  <Alert severity="warning" sx={{ py: 0 }}>
                    {t('notifications.settings.privateUrlWarning')}
                  </Alert>
                )}
                <Box>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={<PlayArrowIcon />}
                    onClick={() => handleTest('ntfy')}
                    disabled={
                      testing === 'ntfy' || !notifyNtfy || !ntfyUrl.trim() || !ntfyTopic.trim()
                    }
                  >
                    {testing === 'ntfy'
                      ? t('notifications.settings.testing')
                      : t('notifications.settings.test')}
                  </Button>
                </Box>
              </Stack>
            </Box>

            {/* Gotify */}
            <Divider />
            <Box>
              <FormControlLabel
                control={
                  <Switch
                    checked={notifyGotify}
                    onChange={(e) => setNotifyGotify(e.target.checked)}
                    size="small"
                  />
                }
                label={t('notifications.settings.gotify.enable')}
              />
              <Stack spacing={1.5} sx={{ mt: 1 }}>
                <TextField
                  label={t('notifications.settings.gotify.url')}
                  value={gotifyUrl}
                  onChange={(e) => {
                    setGotifyUrl(e.target.value);
                    setSaveError('');
                    setTestResult(null);
                  }}
                  fullWidth
                  size="small"
                  placeholder="http://gotify:8080"
                  helperText={t('notifications.settings.gotify.urlHint')}
                />
                <TextField
                  label={t('notifications.settings.gotify.token')}
                  type="password"
                  value={gotifyToken}
                  onChange={(e) => {
                    setGotifyToken(e.target.value);
                    setSaveError('');
                    setTestResult(null);
                  }}
                  fullWidth
                  size="small"
                  helperText={
                    config?.gotify_has_token
                      ? t('notifications.settings.gotify.tokenHintExisting')
                      : t('notifications.settings.gotify.tokenHintNew')
                  }
                />
                {gotifyPrivate && (
                  <Alert severity="warning" sx={{ py: 0 }}>
                    {t('notifications.settings.privateUrlWarning')}
                  </Alert>
                )}
                <Box>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={<PlayArrowIcon />}
                    onClick={() => handleTest('gotify')}
                    disabled={
                      testing === 'gotify' ||
                      !notifyGotify ||
                      !gotifyUrl.trim() ||
                      !gotifyToken.trim()
                    }
                  >
                    {testing === 'gotify'
                      ? t('notifications.settings.testing')
                      : t('notifications.settings.test')}
                  </Button>
                </Box>
              </Stack>
            </Box>

            {/* Web Push */}
            <Divider />
            <Box>
              <FormControlLabel
                control={
                  <Switch
                    checked={notifyPush}
                    onChange={(e) => setNotifyPush(e.target.checked)}
                    size="small"
                  />
                }
                label={t('notifications.settings.push.enable')}
              />
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                {t('notifications.settings.push.description')}
              </Typography>
              <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<NotificationsIcon />}
                  onClick={handleEnablePush}
                  disabled={enablingPush || !browserSupportsPush()}
                >
                  {enablingPush
                    ? t('notifications.settings.push.enabling')
                    : t('notifications.settings.push.enableButton')}
                </Button>
              </Stack>

              <Box sx={{ mt: 1.5 }}>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    mb: 0.5,
                  }}
                >
                  <Typography variant="caption" color="text.secondary">
                    {t('notifications.settings.push.devices')}
                  </Typography>
                  {subsLoading && <CircularProgress size={14} />}
                </Box>
                {subscriptions.length === 0 ? (
                  <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                    {t('notifications.settings.push.noDevices')}
                  </Typography>
                ) : (
                  <List dense sx={{ py: 0 }}>
                    {subscriptions.map((sub) => (
                      <ListItem
                        key={sub.id}
                        sx={{ px: 0, py: 0.5 }}
                        secondaryAction={
                          <IconButton
                            edge="end"
                            size="small"
                            color="error"
                            onClick={() => handleDeleteDevice(sub)}
                            title={t('notifications.settings.push.removeDevice')}
                            aria-label={t('notifications.settings.push.removeDevice')}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        }
                      >
                        <ListItemText
                          primary={sub.device_label || 'Browser'}
                          primaryTypographyProps={{ component: 'div', variant: 'body2' }}
                          secondary={new Date(sub.created_at).toLocaleString()}
                          secondaryTypographyProps={{ component: 'div', variant: 'caption' }}
                        />
                      </ListItem>
                    ))}
                  </List>
                )}
                <Box sx={{ mt: 1 }}>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={<PlayArrowIcon />}
                    onClick={() => handleTest('push')}
                    disabled={
                      testing === 'push' ||
                      !notifyPush ||
                      (subscriptions.length === 0 && devices.length === 0)
                    }
                  >
                    {testing === 'push'
                      ? t('notifications.settings.testing')
                      : t('notifications.settings.test')}
                  </Button>
                </Box>
              </Box>

              <Box sx={{ mt: 1.5 }}>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    mb: 0.5,
                  }}
                >
                  <Typography variant="caption" color="text.secondary">
                    {t('notifications.settings.push.mobileDevicesTitle')}
                  </Typography>
                  {devicesLoading && <CircularProgress size={14} />}
                </Box>
                {devices.length === 0 ? (
                  <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                    {t('notifications.settings.push.noMobileDevices')}
                  </Typography>
                ) : (
                  <List dense sx={{ py: 0 }}>
                    {devices.map((device) => (
                      <ListItem
                        key={device.id}
                        sx={{ px: 0, py: 0.5 }}
                        secondaryAction={
                          <IconButton
                            edge="end"
                            size="small"
                            color="error"
                            onClick={() => handleDeleteMobileDevice(device)}
                            title={t('notifications.settings.push.removeDevice')}
                            aria-label={t('notifications.settings.push.removeDevice')}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        }
                      >
                        <PhoneIphoneIcon fontSize="small" sx={{ mr: 1, color: 'text.secondary' }} />
                        <ListItemText
                          primary={device.device_label || device.client.toUpperCase()}
                          primaryTypographyProps={{ component: 'div', variant: 'body2' }}
                          secondary={new Date(device.created_at).toLocaleString()}
                          secondaryTypographyProps={{ component: 'div', variant: 'caption' }}
                        />
                      </ListItem>
                    ))}
                  </List>
                )}
              </Box>
            </Box>

            {testResult && (
              <Alert severity={testResult.ok ? 'success' : 'warning'} sx={{ py: 0 }}>
                {testResult.ok
                  ? t('notifications.settings.testSuccess', {
                      channel: t(`notifications.settings.channels.${testResult.channel}`),
                    })
                  : t('notifications.settings.testFailed', {
                      error: testResult.error || 'unknown',
                    })}
              </Alert>
            )}

            <Box>
              <Button variant="contained" size="small" onClick={handleSave} disabled={saving}>
                {saving ? t('common.saving') : t('notifications.settings.saveButton')}
              </Button>
            </Box>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
