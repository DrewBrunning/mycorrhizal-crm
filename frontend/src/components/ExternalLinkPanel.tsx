import { mdiImageMultipleOutline, mdiLinkVariant } from '@mdi/js';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import SyncIcon from '@mui/icons-material/Sync';
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  Link,
  Paper,
  Stack,
  SvgIcon,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ExternalIdentity } from '../api/externalLinks';
import { type ImmichPerson, type ImmichPersonSummary, immichThumbnailUrl } from '../api/immich';
import type { WebDAVItem } from '../api/nextcloud';
import type { PaperlessDocument } from '../api/paperless';
import type { SeafileItem, SeafileLibrary } from '../api/seafile';
import { useDateFormat } from '../DateFormatProvider';
import { isHttpUrlString } from '../utils/linkResolution';
import AuthImg from './AuthImg';
import FileLinksPanel, { type FileSystem } from './FileLinksPanel';
import ImmichPersonSearchDialog from './ImmichPersonSearchDialog';
import type { SeafileLinkTarget } from './SeafileFilePickerDialog';

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
  // File-sharing integrations (P2a/P2b/P2c): which systems are configured and
  // the browse/link/unlink callbacks that power their pickers.
  fileSystemsConfigured?: { paperless: boolean; seafile: boolean; nextcloud: boolean };
  onFetchPaperlessDocuments?: (query?: string) => Promise<PaperlessDocument[]>;
  onLinkPaperless?: (doc: PaperlessDocument) => Promise<void>;
  onFetchSeafileLibraries?: () => Promise<SeafileLibrary[]>;
  onFetchSeafileDir?: (repoId: string, path: string) => Promise<SeafileItem[]>;
  onLinkSeafile?: (target: SeafileLinkTarget) => Promise<void>;
  onFetchNextcloudDir?: (path?: string) => Promise<WebDAVItem[]>;
  onLinkNextcloud?: (item: WebDAVItem) => Promise<void>;
  onUnlinkFileSystem?: (system: FileSystem, identityId: string) => Promise<void>;
}

// ExternalLinkPanel is the contact page's "External links" surface (T14 + the
// T15/T16 Immich surface + the P2a/P2b/P2c file-sharing surfaces). The
// substrate's ExternalIdentity list is generic — Immich is rendered as a rich
// row, the file-sharing integrations render through FileLinksPanel, and every
// other system falls back to the generic list. Later integrations slot into
// the same generic list without schema changes.
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
  fileSystemsConfigured,
  onFetchPaperlessDocuments,
  onLinkPaperless,
  onFetchSeafileLibraries,
  onFetchSeafileDir,
  onLinkSeafile,
  onFetchNextcloudDir,
  onLinkNextcloud,
  onUnlinkFileSystem,
}: ExternalLinkPanelProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [searchOpen, setSearchOpen] = useState(false);
  const [error, setError] = useState('');

  const immichIdentity = identities.find((i) => i.system === 'immich');
  const fileSystems = new Set(['paperless', 'seafile', 'nextcloud']);
  const otherIdentities = identities.filter(
    (i) => i.system !== 'immich' && !fileSystems.has(i.system),
  );

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
              <SvgIcon color="action">
                <path d={mdiImageMultipleOutline} />
              </SvgIcon>
              <Typography variant="subtitle2" component="h3">
                {t('immich.panel.notLinkedTitle')}
              </Typography>
            </Box>
            <Button
              size="small"
              variant="contained"
              color="primary"
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
              <AuthImg
                src={immichThumbnailUrl(contactUid)}
                alt=""
                sx={{
                  width: 56,
                  height: 56,
                  borderRadius: 1,
                  objectFit: 'cover',
                  bgcolor: 'action.hover',
                }}
              />
            )}
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                <Typography variant="subtitle2" component="h3" sx={{ fontWeight: 600 }}>
                  {immichSummary?.person_name ||
                    (typeof immichIdentity.metadata?.person_name === 'string'
                      ? immichIdentity.metadata.person_name
                      : '') ||
                    t('immich.panel.linkedPerson')}
                </Typography>
                <Chip size="small" label="Immich" />
                {immichIdentity.url &&
                  (isHttpUrlString(immichIdentity.url) ? (
                    <Link
                      href={immichIdentity.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25 }}
                    >
                      {t('immich.panel.openInImmich')} <OpenInNewIcon sx={{ fontSize: 14 }} />
                    </Link>
                  ) : (
                    // A URL that predates the write-time `httpurl` validator
                    // (or arrived via a non-API path) is shown as text, never
                    // turned into an href (T41).
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ overflowWrap: 'anywhere' }}
                    >
                      {immichIdentity.url}
                    </Typography>
                  ))}
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                {t('immich.panel.photoCount', { count: immichSummary?.photo_count ?? 0 })}
              </Typography>
              {immichSummary?.latest_at && (
                <Typography variant="body2" color="text.secondary">
                  {t('immich.panel.latestAppearance', {
                    date: formatDate(immichSummary.latest_at),
                  })}
                </Typography>
              )}
              <Box sx={{ display: 'flex', gap: 0.5, mt: 1 }}>
                <IconButton
                  size="small"
                  title={t('immich.panel.syncNow')}
                  aria-label={t('immich.panel.syncNow')}
                  onClick={onSyncImmich}
                  disabled={syncing}
                >
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

      {error && (
        <Alert severity="error" sx={{ py: 0 }}>
          {error}
        </Alert>
      )}

      <FileLinksPanel
        identities={identities}
        configured={fileSystemsConfigured ?? { paperless: false, seafile: false, nextcloud: false }}
        onFetchPaperlessDocuments={onFetchPaperlessDocuments}
        onLinkPaperless={onLinkPaperless}
        onFetchSeafileLibraries={onFetchSeafileLibraries}
        onFetchSeafileDir={onFetchSeafileDir}
        onLinkSeafile={onLinkSeafile}
        onFetchNextcloudDir={onFetchNextcloudDir}
        onLinkNextcloud={onLinkNextcloud}
        onUnlink={onUnlinkFileSystem}
      />

      {otherIdentities.length > 0 && (
        <>
          <Divider />
          <Typography variant="subtitle2" component="h3" color="text.secondary">
            {t('externalLinks.otherLinks')}
          </Typography>
          {loading ? (
            <CircularProgress size={24} />
          ) : (
            otherIdentities.map((identity) => (
              <Paper key={identity.id} variant="outlined" sx={{ p: 1.5 }}>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 1,
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                    <SvgIcon color="action" fontSize="small">
                      <path d={mdiLinkVariant} />
                    </SvgIcon>
                    <Box sx={{ minWidth: 0 }}>
                      <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                        {identity.system}
                      </Typography>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ overflowWrap: 'anywhere' }}
                      >
                        {identity.external_id}
                      </Typography>
                    </Box>
                  </Box>
                  {identity.url &&
                    (isHttpUrlString(identity.url) ? (
                      <Link href={identity.url} target="_blank" rel="noopener noreferrer">
                        <OpenInNewIcon fontSize="small" />
                      </Link>
                    ) : (
                      // Same render-time guard as the Immich row: an unsafe
                      // or non-http URL is shown as text, not as an href.
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ overflowWrap: 'anywhere', maxWidth: 240 }}
                      >
                        {identity.url}
                      </Typography>
                    ))}
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
