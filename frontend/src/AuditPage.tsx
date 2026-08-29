import ClearIcon from '@mui/icons-material/Clear';
import DownloadIcon from '@mui/icons-material/Download';
import UndoIcon from '@mui/icons-material/Undo';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import type { SelectChangeEvent } from '@mui/material/Select';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import {
  AUDIT_ENTITY_TYPES,
  type AuditEntityType,
  type AuditEvent,
  exportAuditLog,
} from './api/audit';
import { ApiError } from './api/client';
import { getContactsByUid } from './api/contacts';
import { ListSkeleton } from './components/LoadingSkeletons';
import { useSnackbar } from './context/SnackbarContext';
import { useAudit } from './hooks/useAudit';
import { useDebouncedValue } from './hooks/useDebounce';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { getErrorMessage } from './utils/errorHandler';

const OPERATION_COLORS: Record<AuditEvent['operation'], 'success' | 'info' | 'error'> = {
  create: 'success',
  update: 'info',
  delete: 'error',
  // Auth/admin lifecycle events (issue #381) — failures are errors, the rest
  // are informational.
  login: 'info',
  login_failed: 'error',
  register: 'success',
  password_change: 'info',
  password_reset: 'info',
  password_reset_requested: 'info',
  totp_enable: 'info',
  totp_disable: 'info',
  recovery_regenerate: 'info',
  revoke: 'error',
  role_change: 'info',
};

// T60: the read-only audit
// log backed by T18's already-shipped GET /audit. Reverse-chronological list
// with an entity-type/entity-id filter toolbar and a Contact-only Undo
// affordance on update events (POST /audit/:id/undo; every other entity or a
// delete event is 400 server-side, so the button is gated to match).
export default function AuditPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.audit'));
  const { showSuccess, showError } = useSnackbar();

  const {
    events,
    loading,
    error,
    entityType,
    applyEntityType,
    applyEntityId,
    clearFilters,
    handleUndo,
    canLoadMore,
    loadMore,
  } = useAudit();

  // The entity_id field filters server-side but is debounced so every keystroke
  // doesn't fire a request.
  const [entityIdInput, setEntityIdInput] = useState('');
  const debouncedEntityId = useDebouncedValue(entityIdInput, 350);
  useEffect(() => {
    applyEntityId(debouncedEntityId);
  }, [debouncedEntityId, applyEntityId]);

  const [pendingUndo, setPendingUndo] = useState<AuditEvent | null>(null);
  const [undoing, setUndoing] = useState(false);
  const [exportingAuditLog, setExportingAuditLog] = useState(false);

  // Resolve the event's contact vcard_uids to (numeric-ID, name) pairs so the
  // entity cell links to /contacts/:id when the contact still exists. The API
  // includes archived contacts but not deleted ones, so a deleted contact falls
  // back to its raw uid as plain text.
  const contactsByUid = useContactsForEvents(events);
  const hasFilters = entityType !== '' || entityIdInput.trim() !== '';

  const handleEntityTypeChange = (event: SelectChangeEvent<string>) => {
    applyEntityType(event.target.value as AuditEntityType | '');
  };

  const handleClearFilters = () => {
    setEntityIdInput('');
    clearFilters();
  };

  // Issue #416. Unbounded CSV export of the caller's own full audit trail
  // (unlike the paginated list above) -- before_snapshot is deliberately
  // left out (see api/audit.ts's exportAuditLog doc comment), so this button
  // never offers the sensitive-opt-in variant.
  const handleExportAuditLog = async () => {
    setExportingAuditLog(true);
    try {
      await exportAuditLog();
      showSuccess(t('audit.export.success'));
    } catch (err) {
      showError(getErrorMessage(err));
    } finally {
      setExportingAuditLog(false);
    }
  };

  const handleUndoConfirm = async () => {
    if (!pendingUndo) return;
    setUndoing(true);
    try {
      await handleUndo(pendingUndo.id);
      showSuccess(t('audit.undo.success'));
      setPendingUndo(null);
    } catch (err) {
      // 410: the event has aged past AUDIT_RETENTION_DAYS and the purge has
      // removed it -- there is nothing left to undo. Every other failure shows
      // the server's own message (400 delete/unsupported entity, 404 gone).
      if (err instanceof ApiError && err.status === 410) {
        showError(t('audit.undo.retentionGone'));
      } else {
        showError(getErrorMessage(err));
      }
    } finally {
      setUndoing(false);
    }
  };

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" component="h1" gutterBottom sx={{ mb: 1.5 }}>
        {t('audit.title')}
      </Typography>
      <Typography
        variant="body2"
        sx={{
          color: 'text.secondary',
          mb: 2,
        }}
      >
        {t('audit.description')}
      </Typography>

      <Paper sx={{ p: 1.5, mb: 2 }}>
        <Box
          sx={{
            display: 'flex',
            gap: 2,
            flexWrap: 'wrap',
            alignItems: 'center',
          }}
        >
          <FormControl size="small" sx={{ minWidth: 180 }}>
            <InputLabel id="audit-entity-type-label">{t('audit.filters.entityType')}</InputLabel>
            <Select
              labelId="audit-entity-type-label"
              label={t('audit.filters.entityType')}
              value={entityType}
              onChange={handleEntityTypeChange}
            >
              <MenuItem value="">{t('audit.filters.entityTypeAll')}</MenuItem>
              {AUDIT_ENTITY_TYPES.map((type) => (
                <MenuItem key={type} value={type}>
                  {t(`audit.entityTypes.${type}`)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <TextField
            size="small"
            label={t('audit.filters.entityId')}
            value={entityIdInput}
            onChange={(e) => setEntityIdInput(e.target.value)}
            variant="outlined"
            sx={{ flex: 1, minWidth: 200 }}
          />
          <Button
            variant="outlined"
            size="medium"
            startIcon={<ClearIcon />}
            onClick={handleClearFilters}
            disabled={!hasFilters}
          >
            {t('audit.filters.clear')}
          </Button>
          <Button
            variant="outlined"
            size="medium"
            startIcon={
              exportingAuditLog ? <CircularProgress size={16} color="inherit" /> : <DownloadIcon />
            }
            onClick={handleExportAuditLog}
            disabled={exportingAuditLog}
            sx={{ ml: 'auto' }}
          >
            {exportingAuditLog ? t('audit.export.exporting') : t('audit.export.downloadButton')}
          </Button>
        </Box>
      </Paper>

      {error && (
        <Paper sx={{ p: 2, mb: 2 }}>
          <Typography color="error">{error}</Typography>
        </Paper>
      )}

      {loading && events.length === 0 ? (
        <ListSkeleton count={8} />
      ) : events.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography
            variant="body1"
            sx={{
              color: 'text.secondary',
            }}
          >
            {hasFilters ? t('audit.empty') : t('audit.emptyNoFilters')}
          </Typography>
        </Paper>
      ) : (
        <TableContainer component={Paper}>
          <Table size="small" sx={{ minWidth: 640 }}>
            <TableHead>
              <TableRow>
                <TableCell>{t('audit.columns.timestamp')}</TableCell>
                <TableCell>{t('audit.columns.entityType')}</TableCell>
                <TableCell>{t('audit.columns.operation')}</TableCell>
                <TableCell>{t('audit.columns.entityId')}</TableCell>
                <TableCell align="right">{t('audit.columns.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {events.map((event) => (
                <TableRow key={event.id} hover>
                  <TableCell>
                    <Typography variant="body2">{formatDateTime(event.created_at)}</Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      variant="outlined"
                      label={t(`audit.entityTypes.${event.entity_type}`)}
                    />
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      color={OPERATION_COLORS[event.operation]}
                      label={t(`audit.operations.${event.operation}`)}
                    />
                  </TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>
                    <EntityIdCell event={event} contact={contactsByUid.get(event.entity_id)} />
                  </TableCell>
                  <TableCell align="right">
                    {event.operation === 'update' && event.entity_type === 'contact' && (
                      <Button
                        size="small"
                        color="inherit"
                        startIcon={<UndoIcon />}
                        onClick={() => setPendingUndo(event)}
                      >
                        {t('audit.undo.button')}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {canLoadMore && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            mt: 3,
          }}
        >
          <Button variant="outlined" onClick={loadMore} disabled={loading}>
            {t('common.loadMore')}
          </Button>
        </Box>
      )}

      <Dialog open={!!pendingUndo} onClose={() => setPendingUndo(null)}>
        <DialogTitle>{t('audit.undo.confirmTitle')}</DialogTitle>
        <DialogContent>
          <Typography>
            {pendingUndo &&
              t('audit.undo.confirmMessage', {
                entity: t(`audit.entityTypes.${pendingUndo.entity_type}`),
                date: formatDateTime(pendingUndo.created_at),
              })}
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
              mt: 1,
            }}
          >
            {t('audit.undo.partialNote')}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPendingUndo(null)} disabled={undoing}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={handleUndoConfirm}
            disabled={undoing}
          >
            {t('audit.undo.confirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

// Fetches the numeric ID + display name for the events' contact uids. This is
// the only entity with a navigable per-entity route, so only contact events
// resolve to a link; every other entity_type renders its raw ID.
//
// The uid set is stringified as a sorted-comma key so the effect only re-fires
// when the actual set of contact uids changes — not on every events-array
// mutation (Load more, filter change, undo refresh).
function useContactsForEvents(events: AuditEvent[]) {
  const [contactsByUid, setContactsByUid] = useState<Map<string, { ID: number; name: string }>>(
    new Map(),
  );

  // A stable string key derived from the sorted unique uid set. Two consecutive
  // events arrays that carry the same contact uids produce the same string,
  // which Object.is treats as equal → the effect does not re-fire (the only way
  // to skip the batch GET /contacts?… call when nothing changed without a ref).
  const contactUidsKey = useMemo(() => {
    const uids = events.filter((e) => e.entity_type === 'contact').map((e) => e.entity_id);
    if (uids.length === 0) return '';
    return [...new Set(uids)].sort().join(',');
  }, [events]);

  useEffect(() => {
    if (!contactUidsKey) {
      setContactsByUid(new Map());
      return;
    }
    const contactUids = contactUidsKey.split(',');
    let cancelled = false;
    getContactsByUid(contactUids)
      .then((byUid) => {
        if (cancelled) return;
        const next = new Map<string, { ID: number; name: string }>();
        for (const [uid, contact] of byUid) {
          next.set(uid, {
            ID: contact.ID,
            name: `${contact.firstname} ${contact.lastname}`.trim(),
          });
        }
        setContactsByUid(next);
      })
      .catch(() => {
        // Resolution is a nicety -- the raw uid is still rendered, so a
        // failure here degrades to plain text rather than an error.
      });
    return () => {
      cancelled = true;
    };
  }, [contactUidsKey]);

  return contactsByUid;
}

function EntityIdCell({
  event,
  contact,
}: {
  event: AuditEvent;
  contact?: { ID: number; name: string };
}) {
  const { t } = useTranslation();
  if (event.entity_type === 'contact' && contact) {
    return (
      <Typography
        variant="body2"
        component={RouterLink}
        to={`/contacts/${contact.ID}`}
        sx={{
          textDecoration: 'none',
          color: 'primary.main',
          // 2.5.8 Target Size (Minimum): this link is the sole content of its
          // <td>, not inline prose, so it needs its own 24px target -- body2's
          // line-height alone renders an 18px-tall clickable area.
          display: 'inline-flex',
          alignItems: 'center',
          minHeight: 24,
        }}
      >
        {contact.name || t('audit.contactUnnamed')}
      </Typography>
    );
  }
  return (
    <Typography
      variant="body2"
      component="span"
      sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}
    >
      {event.entity_id}
    </Typography>
  );
}

// Audit timestamps need date + time (multiple events can share a day), so this
// uses the user's locale rather than the date-only DateFormat preference.
function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}
