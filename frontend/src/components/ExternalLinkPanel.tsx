import { useState } from 'react';
import {
  Box,
  Typography,
  Stack,
  Paper,
  Button,
  IconButton,
  Chip,
  Link,
  Divider,
  CircularProgress,
  Alert,
} from '@mui/material';
import { mdiLinkVariant, mdiImageMultipleOutline } from '@mdi/js';
import { SvgIcon } from '@mui/material';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import SyncIcon from '@mui/icons-material/Sync';
import { useTranslation } from 'react-i18next';
import { ExternalIdentity } from '../api/externalLinks';
import { ImmichPerson, ImmichPersonSummary, immichThumbnailUrl } from '../api/immich';
import ImmichPersonSearchDialog from './ImmichPersonSearchDialog';
import { useDateFormat } from '../DateFormatProvider';

interface ExternalLinkPanelProps {
  contactUid: string;
  identities: ExternalIdentity[];
  loading?: boolean;
  // Immich-specific surface (the first integration on the substrate).
  immichSummary?: ImmichPersonSummary | null;
  immichSummaryLoading?: boolean;
  onFetchImmichPeople: () => Promise<ImmichPerson[]>;
  onLinkImmich: (person: ImmichPerson) => Promise<void>;
  onUnlinkImmich: () => Promise<void>;
  onSyncImmich: () => Promise<void>;
  syncing?: boolean;
}

// ExternalLinkPanel is the contact page's "External links" surface (T14 + the
// T15/T16 Immich surface). The substrate's ExternalIdentity list is generic —
// Immich is currently the only integration, rendered as a richer row with the
// live person summary (photo count, latest appearance, deep link, thumbnail)
// and the link/unlink flow. Later integrations slot into the same generic
// list without schema changes.
export default function ExternalLinkPanel({
  contactUid,
  identities,
  loading = false,
  immichSummary,
  immichSummaryLoading = false,
  onFetchImmichPeople,
  onLinkImmich,
  onUnlinkImmich,
  onSyncImmich,
  syncing = false,
}: ExternalLinkPanelProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [searchOpen, setSearchOpen] = useState(false);
  const [error, setError] = useState('');
  // The proxied thumbnail may 404/503 (Immich down, or the link's person
  // removed upstream). Hide the broken <img> rather than showing an error
  // icon — "degrade to no photos rather than erroring" (T16 trap).
  const [thumbFailed, setThumbFailed] = useState(false);

  const immichIdentity = identities.find((i) => i.system === 'immich');

  const handleLink = async (person: ImmichPerson) => {
    try {
      await onLinkImmich(person);
      setError('');
    } catch (err) {
      setError(err instanceof Error ? err.message : t('immich.search.linkFailed'));
      throw err;
    }
  };

  const handleUnlink = async () => {
    if (!window.confirm(t('immich.panel.unlinkConfirm'))) return;
    try {
      await onUnlinkImmich();
      setError('');
    } catch (err) {
      setError(err instanceof Error ? err.message : t('immich.panel.unlinkFailed'));
    }
  };

  return (
    <Stack spacing={1.5}>
      {!immichIdentity && (
        <Paper variant="outlined" sx={{ p: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <SvgIcon color="action"><path d={mdiImageMultipleOutline} /></SvgIcon>
              <Typography variant="subtitle2">{t('immich.panel.notLinkedTitle')}</Typography>
            </Box>
            <Button
              size="small"
              variant="outlined"
              startIcon={<AddIcon />}
              onClick={() => setSearchOpen(true)}
            >
              {t('immich.panel.linkButton')}
            </Button>
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            {t('immich.panel.notLinkedHint')}
          </Typography>
        </Paper>
      )}

      {immichIdentity && (
        <Paper variant="outlined" sx={{ p: 2 }}>
          <Box sx={{ display: 'flex', gap: 2 }}>
            {immichSummaryLoading ? (
              <CircularProgress size={40} />
            ) : (
              !thumbFailed && (
                <Box
                  component="img"
                  src={immichThumbnailUrl(contactUid)}
                  alt=""
                  onError={() => setThumbFailed(true)}
                  sx={{ width: 56, height: 56, borderRadius: 1, objectFit: 'cover', bgcolor: 'action.hover' }}
                />
              )
            )}
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                  {immichSummary?.person_name ||
                    (typeof immichIdentity.metadata?.person_name === 'string' ? immichIdentity.metadata.person_name : '') ||
                    t('immich.panel.linkedPerson')}
                </Typography>
                <Chip size="small" label="Immich" />
                {immichIdentity.url && (
                  <Link
                    href={immichIdentity.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25 }}
                  >
                    {t('immich.panel.openInImmich')} <OpenInNewIcon sx={{ fontSize: 14 }} />
                  </Link>
                )}
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                {t('immich.panel.photoCount', { count: immichSummary?.photo_count ?? 0 })}
              </Typography>
              {immichSummary?.latest_at && (
                <Typography variant="body2" color="text.secondary">
                  {t('immich.panel.latestAppearance', { date: formatDate(immichSummary.latest_at) })}
                </Typography>
              )}
              <Box sx={{ display: 'flex', gap: 0.5, mt: 1 }}>
                <IconButton size="small" title={t('immich.panel.syncNow')} onClick={onSyncImmich} disabled={syncing}>
                  <SyncIcon fontSize="small" />
                </IconButton>
                <IconButton
                  size="small"
                  color="error"
                  title={t('immich.panel.unlink')}
                  onClick={handleUnlink}
                  aria-label={t('immich.panel.unlink')}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Box>
            </Box>
          </Box>
        </Paper>
      )}

      {error && <Alert severity="error" sx={{ py: 0 }}>{error}</Alert>}

      {identities.length > 0 && (
        <>
          <Divider />
          <Typography variant="subtitle2" color="text.secondary">
            {t('externalLinks.otherLinks')}
          </Typography>
          {loading ? (
            <CircularProgress size={24} />
          ) : (
            identities
              .filter((i) => i.system !== 'immich')
              .map((identity) => (
                <Paper key={identity.id} variant="outlined" sx={{ p: 1.5 }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                      <SvgIcon color="action" fontSize="small"><path d={mdiLinkVariant} /></SvgIcon>
                      <Box sx={{ minWidth: 0 }}>
                        <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                          {identity.system}
                        </Typography>
                        <Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
                          {identity.external_id}
                        </Typography>
                      </Box>
                    </Box>
                    {identity.url && (
                      <Link href={identity.url} target="_blank" rel="noopener noreferrer">
                        <OpenInNewIcon fontSize="small" />
                      </Link>
                    )}
                  </Box>
                </Paper>
              ))
          )}
        </>
      )}

      <ImmichPersonSearchDialog
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        onFetchPeople={onFetchImmichPeople}
        onSelect={handleLink}
      />
    </Stack>
  );
}
