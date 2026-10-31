import ContactsIcon from '@mui/icons-material/Contacts';
import {
  Alert,
  Autocomplete,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type CardDAVSyncScope,
  type CardDAVSyncScopeInput,
  type CardDAVSyncScopeMode,
  type CardDAVSyncScopePreview,
  getCardDAVSyncScope,
  previewCardDAVSyncScope,
  putCardDAVSyncScope,
} from '../api/carddavSyncScope';
import { type Circle, listCircles } from '../api/circles';
import { getHealth } from '../api/health';
import { useSnackbar } from '../context/SnackbarContext';

async function loadAllCircles(): Promise<Circle[]> {
  const circles: Circle[] = [];
  let cursor = '';
  do {
    const page = await listCircles({
      cursor: cursor || undefined,
      limit: 100,
      include_members: false,
    });
    circles.push(...page.circles);
    cursor = page.next_cursor;
  } while (cursor);
  return circles;
}

function sameIDs(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false;
  const sorted = [...a].sort();
  return [...b].sort().every((id, index) => id === sorted[index]);
}

// Issue #1993 / ADR 0041: choose which circles the single CardDAV address book
// serves. The default is "all contacts", which is byte-for-byte today's
// behaviour. Android and the REST API are unaffected.
export default function CardDAVSyncScopeSettings() {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [scope, setScope] = useState<CardDAVSyncScope | null>(null);
  const [circles, setCircles] = useState<Circle[]>([]);
  const [mode, setMode] = useState<CardDAVSyncScopeMode>('all');
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
  const [defaultCircleID, setDefaultCircleID] = useState<string | null>(null);
  const [preview, setPreview] = useState<CardDAVSyncScopePreview | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const health = await getHealth();
        if (!health.capabilities?.includes('carddav')) {
          if (!cancelled) setEnabled(false);
          return;
        }
        const [loaded, allCircles] = await Promise.all([getCardDAVSyncScope(), loadAllCircles()]);
        if (cancelled) return;
        setEnabled(true);
        setScope(loaded);
        setCircles(allCircles);
        setMode(loaded.mode);
        setSelectedIDs(loaded.circle_ids);
        setDefaultCircleID(loaded.default_circle_id);
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : t('carddavSyncScope.loadError'));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [t]);

  const input = useMemo<CardDAVSyncScopeInput>(
    () => ({
      mode,
      circle_ids: mode === 'all' ? [] : selectedIDs,
      default_circle_id: mode === 'all' ? null : defaultCircleID,
    }),
    [mode, selectedIDs, defaultCircleID],
  );

  const dirty =
    scope !== null &&
    (mode !== scope.mode ||
      !sameIDs(input.circle_ids, scope.circle_ids) ||
      input.default_circle_id !== scope.default_circle_id);

  useEffect(() => {
    if (!enabled || !dirty) {
      setPreview(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      void previewCardDAVSyncScope(input)
        .then((next) => {
          if (!cancelled) setPreview(next);
        })
        .catch(() => {
          if (!cancelled) setPreview(null);
        });
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [enabled, dirty, input]);

  const allowedDefaultCircles = useMemo(() => {
    if (mode === 'include') return circles.filter((c) => selectedIDs.includes(c.id));
    if (mode === 'exclude') return circles.filter((c) => !selectedIDs.includes(c.id));
    return [];
  }, [circles, mode, selectedIDs]);

  const invalid =
    mode === 'include' &&
    (selectedIDs.length === 0 ||
      defaultCircleID === null ||
      !selectedIDs.includes(defaultCircleID));

  const handleModeChange = (next: CardDAVSyncScopeMode) => {
    setMode(next);
    if (next === 'all') {
      setSelectedIDs([]);
      setDefaultCircleID(null);
      return;
    }
    if (defaultCircleID) {
      if (next === 'include' && !selectedIDs.includes(defaultCircleID)) setDefaultCircleID(null);
      if (next === 'exclude' && selectedIDs.includes(defaultCircleID)) setDefaultCircleID(null);
    }
  };

  const handleSelectedChange = (ids: string[]) => {
    setSelectedIDs(ids);
    if (mode === 'include') {
      setDefaultCircleID((current) =>
        current && ids.includes(current) ? current : (ids[0] ?? null),
      );
    } else if (mode === 'exclude' && defaultCircleID && ids.includes(defaultCircleID)) {
      setDefaultCircleID(null);
    }
  };

  const handleSave = async () => {
    if (!scope || invalid) return;
    setSaving(true);
    setError('');
    try {
      const latest = await previewCardDAVSyncScope(input);
      const removed = scope.in_scope_count - latest.in_scope_count;
      if (
        removed > 0 &&
        !window.confirm(t('carddavSyncScope.confirmRemoval', { count: removed }))
      ) {
        return;
      }
      const saved = await putCardDAVSyncScope(input);
      setScope(saved);
      setMode(saved.mode);
      setSelectedIDs(saved.circle_ids);
      setDefaultCircleID(saved.default_circle_id);
      setPreview(null);
      showSuccess(t('carddavSyncScope.saveSuccess'));
    } catch (err) {
      setError(err instanceof Error ? err.message : t('carddavSyncScope.saveError'));
    } finally {
      setSaving(false);
    }
  };

  if (!enabled && !loading) return null;

  const warnings = preview?.warnings ?? scope?.warnings ?? [];
  const inScope = preview?.in_scope_count ?? scope?.in_scope_count ?? 0;
  const total = preview?.total_count ?? scope?.total_count ?? 0;

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Stack direction="row" spacing={1} sx={{ mb: 1, alignItems: 'center' }}>
          <ContactsIcon sx={{ color: 'text.secondary', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('carddavSyncScope.title')}
          </Typography>
        </Stack>
        <Divider sx={{ mb: 1.5 }} />
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          {t('carddavSyncScope.description')}
        </Typography>

        {loading && <CircularProgress size={24} />}
        {!loading && (
          <Stack spacing={1.5}>
            {error && <Alert severity="error">{error}</Alert>}
            {warnings.map((warning) => (
              <Alert key={warning.code} severity="warning">
                {t(`carddavSyncScope.warnings.${warning.code}`)}
              </Alert>
            ))}

            <FormControl fullWidth size="small">
              <InputLabel id="carddav-sync-scope-mode-label">
                {t('carddavSyncScope.modeLabel')}
              </InputLabel>
              <Select
                labelId="carddav-sync-scope-mode-label"
                label={t('carddavSyncScope.modeLabel')}
                value={mode}
                onChange={(event) => handleModeChange(event.target.value as CardDAVSyncScopeMode)}
              >
                <MenuItem value="all">{t('carddavSyncScope.modeAll')}</MenuItem>
                <MenuItem value="include">{t('carddavSyncScope.modeInclude')}</MenuItem>
                <MenuItem value="exclude">{t('carddavSyncScope.modeExclude')}</MenuItem>
              </Select>
            </FormControl>

            {mode !== 'all' && (
              <Autocomplete
                multiple
                size="small"
                options={circles}
                getOptionLabel={(circle) => circle.name}
                isOptionEqualToValue={(a, b) => a.id === b.id}
                value={circles.filter((circle) => selectedIDs.includes(circle.id))}
                onChange={(_, value) => handleSelectedChange(value.map((circle) => circle.id))}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label={t(
                      mode === 'include'
                        ? 'carddavSyncScope.includedCircles'
                        : 'carddavSyncScope.excludedCircles',
                    )}
                  />
                )}
              />
            )}

            {mode !== 'all' && (
              <FormControl fullWidth size="small">
                <InputLabel id="carddav-sync-scope-default-label">
                  {t('carddavSyncScope.defaultCircle')}
                </InputLabel>
                <Select
                  labelId="carddav-sync-scope-default-label"
                  label={t('carddavSyncScope.defaultCircle')}
                  value={defaultCircleID ?? ''}
                  onChange={(event) => setDefaultCircleID(event.target.value || null)}
                >
                  {mode === 'exclude' && (
                    <MenuItem value="">{t('carddavSyncScope.noDefaultCircle')}</MenuItem>
                  )}
                  {allowedDefaultCircles.map((circle) => (
                    <MenuItem key={circle.id} value={circle.id}>
                      {circle.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}

            <Typography variant="body2" color="text.secondary">
              {t('carddavSyncScope.counts', { inScope, total })}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {t('carddavSyncScope.help')}
            </Typography>

            <Button
              variant="contained"
              size="small"
              sx={{ alignSelf: 'flex-start' }}
              disabled={!dirty || saving || invalid}
              onClick={() => void handleSave()}
            >
              {saving ? t('carddavSyncScope.saving') : t('carddavSyncScope.save')}
            </Button>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
