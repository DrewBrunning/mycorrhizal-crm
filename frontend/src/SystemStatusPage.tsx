import RefreshIcon from '@mui/icons-material/Refresh';
import {
  Box,
  Button,
  Chip,
  LinearProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { Fragment, type ReactNode, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import {
  type HealthCheckDetail,
  type OverallStatus,
  SYSTEM_STATUS_FEATURE_KEYS,
  type SystemStatusResponse,
} from './api/systemStatus';
import { isAdmin } from './auth';
import { ListSkeleton } from './components/LoadingSkeletons';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useSystemStatus } from './hooks/useSystemStatus';

// Overall roll-up (healthy | degraded | unhealthy) — healthy reads success,
// degraded reads warning, unhealthy reads error. Same idiom as the
// SEVERITY_COLORS map in SystemEventsPage.
const OVERALL_COLORS: Record<OverallStatus, 'success' | 'warning' | 'error'> = {
  healthy: 'success',
  degraded: 'warning',
  unhealthy: 'error',
};

// Per-facet deep-health statuses (ok | degraded | unhealthy | not_configured).
const CHECK_COLORS: Record<string, 'success' | 'warning' | 'error' | 'default'> = {
  ok: 'success',
  degraded: 'warning',
  unhealthy: 'error',
  not_configured: 'default',
};

// Every value on this page degrades to an em dash when its section is missing
// or null — the same rule as BuildVersionCard. A broken facet must never take
// the page down.
function dash(value: unknown): string {
  if (value == null || value === '') return '—';
  return String(value);
}

function formatBytes(bytes?: number): string {
  if (bytes == null) return '—';
  if (bytes < 1024) return `${bytes} B`;
  const units = ['kB', 'MB', 'GB', 'TB'];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(1)} ${units[unit]}`;
}

function formatUptime(seconds?: number): string {
  if (seconds == null) return '—';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0 || d > 0) parts.push(`${h}h`);
  if (m > 0 || d > 0) parts.push(`${m}m`);
  parts.push(`${s}s`);
  return parts.join(' ');
}

function formatDateTime(iso?: string): string {
  if (!iso) return '—';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

// SystemStatusPage — the admin "System status" surface (issue #649): the
// composite operational picture behind GET /admin/system-status. Admin-only,
// read-only. This is the counterpart to the /system-events timeline: that page
// is the "what happened" history, this one is the "what is it like right now"
// snapshot.
export default function SystemStatusPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.systemStatus'));
  const navigate = useNavigate();

  // Self-guard, exactly as UsersPage does: a non-admin is bounced before the
  // (admin-only) snapshot is even requested.
  useEffect(() => {
    if (!isAdmin()) {
      navigate('/');
    }
  }, [navigate]);

  const { data, loading, error, refresh } = useSystemStatus();

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" component="h1" gutterBottom sx={{ mb: 1.5 }}>
        {t('systemStatus.title')}
      </Typography>
      <Typography
        variant="body2"
        sx={{
          color: 'text.secondary',
          mb: 2,
        }}
      >
        {t('systemStatus.description')}
      </Typography>

      {error && (
        <Paper sx={{ p: 2, mb: 2 }}>
          <Typography color="error">{error}</Typography>
        </Paper>
      )}

      {loading && !data ? (
        <ListSkeleton count={6} />
      ) : data ? (
        <Snapshot status={data} onRefresh={refresh} loading={loading} />
      ) : null}
    </Box>
  );
}

function Snapshot({
  status,
  onRefresh,
  loading,
}: {
  status: SystemStatusResponse;
  onRefresh: () => void;
  loading: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Paper sx={{ p: 1.5, mb: 2 }}>
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 1,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 2,
            }}
          >
            <Typography variant="subtitle1" component="h2">
              {t('systemStatus.overall')}
            </Typography>
            <Chip
              size="small"
              color={OVERALL_COLORS[status.overall] ?? 'default'}
              label={t(`systemStatus.status.${status.overall}`)}
            />
          </Box>
          <Button
            size="small"
            startIcon={<RefreshIcon />}
            onClick={onRefresh}
            disabled={loading}
            sx={{ flexShrink: 0 }}
          >
            {t('systemStatus.refresh')}
          </Button>
        </Box>
      </Paper>

      <VersionUptimeCard status={status} />
      <UpdateCard status={status} />
      <HealthChecksCard status={status} />
      <MigrationCard status={status} />
      <ConfigCard status={status} />
      <DatabaseCard status={status} />
      <StorageCard status={status} />
    </>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Paper sx={{ p: 1.5, mb: 2 }}>
      <Typography variant="subtitle1" component="h2" sx={{ mb: 1 }}>
        {title}
      </Typography>
      {children}
    </Paper>
  );
}

function DetailGrid({ rows }: { rows: [string, string][] }) {
  return (
    <Box
      component="dl"
      sx={{ display: 'grid', gridTemplateColumns: 'max-content 1fr', gap: 1, m: 0 }}
    >
      {rows.map(([label, value]) => (
        <Fragment key={label}>
          <Typography
            component="dt"
            variant="body2"
            sx={{
              color: 'text.secondary',
              fontWeight: 600,
              pr: 2,
            }}
          >
            {label}
          </Typography>
          <Typography component="dd" variant="body2" sx={{ m: 0, wordBreak: 'break-word' }}>
            {value}
          </Typography>
        </Fragment>
      ))}
    </Box>
  );
}

function VersionUptimeCard({ status }: { status: SystemStatusResponse }) {
  const { t } = useTranslation();
  const version = status.version;
  return (
    <Section title={t('systemStatus.version.title')}>
      <DetailGrid
        rows={[
          [t('systemStatus.version.version'), dash(version?.version)],
          [t('systemStatus.version.commit'), dash(version?.commit)],
          [t('systemStatus.version.buildDate'), dash(version?.build_date)],
          [t('systemStatus.version.startedAt'), formatDateTime(status.uptime?.started_at)],
          [t('systemStatus.version.uptime'), formatUptime(status.uptime?.uptime_seconds)],
        ]}
      />
    </Section>
  );
}

// UpdateCard renders the opt-in update-availability block (issue #650). It is
// entirely absent when the flag is off or the latest release is unknown —
// the same "informational, render nothing / a dash" rule BuildVersionCard
// follows. Only when a real comparison exists does the card appear, with an
// explicit "Update available" chip when one is found.
function UpdateCard({ status }: { status: SystemStatusResponse }) {
  const { t } = useTranslation();
  const update = status.update;
  if (!update?.enabled || !update.latest) return null;

  return (
    <Section title={t('systemStatus.update.title')}>
      {update.update_available ? (
        <Chip
          color="primary"
          label={t('systemStatus.update.available', { version: update.latest })}
        />
      ) : (
        <Typography
          variant="body2"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('systemStatus.update.upToDate')}
        </Typography>
      )}
      <Box sx={{ mt: 1 }}>
        <DetailGrid
          rows={[
            [t('systemStatus.update.current'), dash(update.current)],
            [t('systemStatus.update.latest'), dash(update.latest)],
            [t('systemStatus.update.checkedAt'), formatDateTime(update.checked_at ?? undefined)],
          ]}
        />
      </Box>
    </Section>
  );
}

function HealthChecksCard({ status }: { status: SystemStatusResponse }) {
  const { t } = useTranslation();
  const health = status.health;

  const facets: { key: string; label: string; detail: HealthCheckDetail }[] = [];
  const fixedFacets = ['database', 'migrations', 'integrity_check', 'restore_drill'] as const;
  for (const facet of fixedFacets) {
    const detail = health?.[facet];
    if (detail) {
      facets.push({
        key: facet,
        label: t(`systemStatus.health.facets.${facet}`),
        detail,
      });
    }
  }
  if (health?.background_jobs) {
    facets.push({
      key: 'background_jobs',
      label: t('systemStatus.health.facets.background_jobs'),
      detail: health.background_jobs,
    });
  }
  if (health?.integrations) {
    for (const [name, detail] of Object.entries(health.integrations)) {
      facets.push({ key: `integrations.${name}`, label: name, detail });
    }
  }

  return (
    <Section title={t('systemStatus.health.title')}>
      <Typography
        variant="body2"
        sx={{
          color: 'text.secondary',
          mb: 1,
        }}
      >
        {t('systemStatus.health.description')}
      </Typography>
      {facets.length === 0 ? (
        <Typography variant="body2">{t('common.none')}</Typography>
      ) : (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>{t('systemStatus.health.facet')}</TableCell>
                <TableCell>{t('systemStatus.health.status')}</TableCell>
                <TableCell>{t('systemStatus.health.reason')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {facets.map((f) => (
                <TableRow key={f.key}>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>{f.label}</TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      variant="outlined"
                      color={CHECK_COLORS[f.detail.status] ?? 'default'}
                      label={t(`systemStatus.status.${f.detail.status}`)}
                    />
                  </TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>
                    {f.detail.reason ? f.detail.reason : '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Section>
  );
}

function MigrationCard({ status }: { status: SystemStatusResponse }) {
  const { t } = useTranslation();
  const migration = status.migration;
  return (
    <Section title={t('systemStatus.migration.title')}>
      <DetailGrid
        rows={[
          [t('systemStatus.migration.applied'), dash(migration?.applied)],
          [t('systemStatus.migration.latest'), dash(migration?.latest)],
          [t('systemStatus.migration.pending'), dash(migration?.pending)],
          [
            t('systemStatus.migration.dirty'),
            migration?.dirty == null
              ? '—'
              : migration.dirty
                ? t('systemStatus.migration.dirtyYes')
                : t('systemStatus.migration.dirtyNo'),
          ],
        ]}
      />
    </Section>
  );
}

function ConfigCard({ status }: { status: SystemStatusResponse }) {
  const { t } = useTranslation();
  const config = status.config;
  const validation = config?.validation ?? [];
  const features = config?.features;

  return (
    <Section title={t('systemStatus.config.title')}>
      <Typography
        variant="body2"
        sx={{
          color: 'text.secondary',
          mb: 1,
        }}
      >
        {t('systemStatus.config.validationTitle')}
      </Typography>

      {validation.length === 0 ? (
        <Typography variant="body2" sx={{ mb: 1.5 }}>
          {t('systemStatus.config.validationPass')}
        </Typography>
      ) : (
        <TableContainer component={Paper} variant="outlined" sx={{ mb: 1.5 }}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>{t('systemStatus.config.field')}</TableCell>
                <TableCell>{t('systemStatus.config.message')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {validation.map((v) => (
                <TableRow key={`${v.field}-${v.message}`}>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>{v.field}</TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>
                    <Typography color="error">{v.message}</Typography>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Typography
        variant="body2"
        sx={{
          color: 'text.secondary',
          mb: 0.5,
        }}
      >
        {t('systemStatus.config.features')}
      </Typography>
      {features ? (
        <Box
          sx={{
            display: 'flex',
            gap: 1,
            flexWrap: 'wrap',
          }}
        >
          {SYSTEM_STATUS_FEATURE_KEYS.map((key) => (
            <Chip
              key={key}
              size="small"
              color={features[key] ? 'primary' : 'default'}
              variant={features[key] ? 'filled' : 'outlined'}
              label={`${t(`systemStatus.features.${key}`)} · ${
                features[key] ? t('systemStatus.feature.on') : t('systemStatus.feature.off')
              }`}
            />
          ))}
        </Box>
      ) : (
        <Typography variant="body2">{t('common.none')}</Typography>
      )}
    </Section>
  );
}

function DatabaseCard({ status }: { status: SystemStatusResponse }) {
  const { t } = useTranslation();
  const database = status.database;
  return (
    <Section title={t('systemStatus.database.title')}>
      <DetailGrid
        rows={[
          [t('systemStatus.database.sqliteVersion'), dash(database?.sqlite_version)],
          [t('systemStatus.database.journalMode'), dash(database?.journal_mode)],
          [t('systemStatus.database.walBytes'), formatBytes(database?.wal_bytes)],
        ]}
      />
    </Section>
  );
}

function StorageCard({ status }: { status: SystemStatusResponse }) {
  const { t } = useTranslation();
  const storage = status.storage;
  const filesystem = storage?.filesystem;
  const total = filesystem?.total_bytes;
  const free = filesystem?.free_bytes;
  const used = total != null && free != null ? total - free : null;
  const usedPct = used != null && total != null && total > 0 ? (used / total) * 100 : null;
  const directories = storage?.directories ?? [];

  // Storage-growth trend (issue #652): the threshold banner colour and the
  // growth / projection line both render em dashes when there is no history.
  const threshold = storage?.threshold ?? 'ok';
  const thresholdColor =
    threshold === 'critical' ? 'error' : threshold === 'warning' ? 'warning' : 'success';

  const growth30 = storage?.growth_30d_bytes ?? null;
  const projectedFull = storage?.projected_full_at ?? null;
  const usage = storage?.usage_percent ?? null;

  return (
    <Section title={t('systemStatus.storage.title')}>
      {threshold !== 'ok' && usage != null && (
        <Chip
          size="small"
          color={thresholdColor}
          label={
            threshold === 'critical'
              ? t('systemStatus.storage.thresholdCritical', { percent: usage })
              : t('systemStatus.storage.thresholdWarning', { percent: usage })
          }
          sx={{ mb: 1 }}
        />
      )}

      <DetailGrid
        rows={[
          [t('systemStatus.storage.databaseSize'), formatBytes(storage?.database_bytes)],
          [
            t('systemStatus.storage.filesystem'),
            used != null && total != null
              ? t('systemStatus.storage.usedOf', {
                  used: formatBytes(used),
                  total: formatBytes(total),
                })
              : '—',
          ],
          [
            t('systemStatus.storage.growth30d'),
            growth30 != null
              ? t('systemStatus.storage.grew', { bytes: formatBytes(growth30) })
              : '—',
          ],
          [
            t('systemStatus.storage.projectedFull'),
            projectedFull ? formatDateTime(projectedFull) : '—',
          ],
        ]}
      />

      {usedPct != null && (
        <Box sx={{ mt: 1.5, mb: 0.5 }}>
          <LinearProgress
            variant="determinate"
            value={Math.min(100, usedPct)}
            color={thresholdColor}
            sx={{ borderRadius: 1, height: 8 }}
          />
        </Box>
      )}
      {free != null && (
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('systemStatus.storage.free')}: {formatBytes(free)}
        </Typography>
      )}

      {directories.length > 0 && (
        <Box sx={{ mt: 1.5 }}>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
              mb: 0.5,
            }}
          >
            {t('systemStatus.storage.directories')}
          </Typography>
          <TableContainer component={Paper} variant="outlined">
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>{t('systemStatus.storage.path')}</TableCell>
                  <TableCell align="right">{t('systemStatus.storage.size')}</TableCell>
                  <TableCell align="right">{t('systemStatus.storage.files')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {directories.map((dir) => (
                  <TableRow key={dir.path}>
                    <TableCell sx={{ overflowWrap: 'anywhere' }}>{dir.path}</TableCell>
                    <TableCell align="right">
                      <Box
                        component="span"
                        sx={{
                          display: 'inline-flex',
                          alignItems: 'center',
                        }}
                      >
                        {formatBytes(dir.bytes)}
                        {dir.truncated && (
                          <Typography
                            component="span"
                            variant="caption"
                            sx={{
                              color: 'text.secondary',
                              ml: 0.5,
                            }}
                          >
                            {t('systemStatus.storage.approx')}
                          </Typography>
                        )}
                      </Box>
                    </TableCell>
                    <TableCell align="right">{dash(dir.file_count)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Box>
      )}
    </Section>
  );
}
