import {
  Alert,
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api/client';
import {
  getShareGrantContent,
  type ShareGrantContent,
  type ShareGrantRecipientView,
} from '../api/shareGrants';
import { useDateFormat } from '../DateFormatProvider';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';
import SharedContactCard from './SharedContactCard';

interface SharedGrantContentDialogProps {
  grant: ShareGrantRecipientView | null;
  onClose: () => void;
}

type State =
  | { status: 'loading' }
  | { status: 'ready'; content: ShareGrantContent }
  | { status: 'unavailable' }
  | { status: 'error'; message: string };

function Field({ label, value }: { label: string; value: string }) {
  if (!value) return null;
  return (
    <Box>
      <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block' }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ overflowWrap: 'anywhere', whiteSpace: 'pre-wrap' }}>
        {value}
      </Typography>
    </Box>
  );
}

// SharedGrantContentDialog is the recipient read view for
// GET /share-grants/{id}/content (ADR 0042 decision 12). Every entity type has a
// dedicated renderer over an explicit field list, mirroring the backend's
// allowlist views: a field the server adds later is not shown until it is added
// here. It is read-only, and re-fetched on every open (the grant can end at any time).
export default function SharedGrantContentDialog({
  grant,
  onClose,
}: SharedGrantContentDialogProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [state, setState] = useState<State>({ status: 'loading' });
  const grantId = grant?.id;

  useEffect(() => {
    if (!grantId) return;
    let cancelled = false;
    setState({ status: 'loading' });
    getShareGrantContent(grantId)
      .then((content) => {
        if (!cancelled) setState({ status: 'ready', content });
      })
      .catch((err) => {
        if (cancelled) return;
        // Every "no longer yours" outcome is the identical 404 (ADR 0042).
        if (err instanceof ApiError && err.status === 404) setState({ status: 'unavailable' });
        else setState({ status: 'error', message: getErrorMessage(err) });
      });
    return () => {
      cancelled = true;
    };
  }, [grantId]);

  const body = (() => {
    if (state.status === 'loading')
      return <CircularProgress size={24} aria-label={t('common.loading')} />;
    if (state.status === 'unavailable') {
      return <Alert severity="info">{t('standingShares.content.unavailable')}</Alert>;
    }
    if (state.status === 'error') return <Alert severity="error">{state.message}</Alert>;
    const { content } = state;
    switch (content.entity_type) {
      case 'contact':
        return content.contact ? (
          <SharedContactCard card={content.contact} />
        ) : (
          <Typography variant="body2">{t('standingShares.content.noDetails')}</Typography>
        );
      case 'note':
        return content.note ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Field label={t('standingShares.content.date')} value={formatDate(content.note.date)} />
            <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
              {content.note.content}
            </Typography>
          </Box>
        ) : null;
      case 'activity': {
        const a = content.activity;
        if (!a) return null;
        return (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Typography variant="subtitle1" component="p" sx={{ fontWeight: 600 }}>
              {a.title}
            </Typography>
            <Field label={t('standingShares.content.date')} value={formatDate(a.date)} />
            <Field
              label={t('standingShares.content.endDate')}
              value={a.end_date ? formatDate(a.end_date) : ''}
            />
            <Field label={t('standingShares.content.type')} value={a.type} />
            <Field label={t('standingShares.content.location')} value={a.location} />
            <Field label={t('standingShares.content.description')} value={a.description} />
          </Box>
        );
      }
      case 'circle': {
        const c = content.circle;
        if (!c) return null;
        return (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Typography variant="subtitle1" component="p" sx={{ fontWeight: 600 }}>
              {c.name}
            </Typography>
            <Field label={t('standingShares.content.description')} value={c.description} />
            <Typography variant="body2">
              {t('standingShares.content.members', { count: c.member_count })}
            </Typography>
            {c.members.length === 0 ? (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('standingShares.content.noMembers')}
              </Typography>
            ) : (
              <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
                {c.members.map((m, i) => (
                  // biome-ignore lint/suspicious/noArrayIndexKey: names are not unique ids; static list
                  <li key={`${i}-${m.display_name}`}>
                    <Typography variant="body2" component="span">
                      {m.display_name}
                    </Typography>
                  </li>
                ))}
              </Box>
            )}
            {c.members_truncated && (
              <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                {t('standingShares.content.membersTruncated', {
                  shown: c.members.length,
                  count: c.member_count,
                })}
              </Typography>
            )}
          </Box>
        );
      }
      default:
        return null;
    }
  })();

  return (
    <AppDialog open={grant !== null} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {grant
          ? t('standingShares.content.title', {
              label: grant.label,
              username: grant.from_username,
            })
          : ''}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
          {state.status === 'ready' && grant && (
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
              {t('standingShares.content.readOnly', { username: grant.from_username })}
            </Typography>
          )}
          {body}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.close')}</Button>
      </DialogActions>
    </AppDialog>
  );
}
