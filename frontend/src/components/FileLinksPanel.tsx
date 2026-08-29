import { mdiCloudOutline, mdiFileDocumentOutline, mdiFolderMultipleOutline } from '@mdi/js';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import FolderIcon from '@mui/icons-material/Folder';
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Alert, Box, Button, IconButton, Link, Paper, Stack, Typography } from '@mui/material';
import type { TFunction } from 'i18next';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ExternalIdentity } from '../api/externalLinks';
import type { WebDAVItem } from '../api/nextcloud';
import type { PaperlessDocument } from '../api/paperless';
import type { SeafileItem, SeafileLibrary } from '../api/seafile';
import { useDateFormat } from '../DateFormatProvider';
import { isHttpUrlString } from '../utils/linkResolution';
import NextcloudFilePickerDialog from './NextcloudFilePickerDialog';
import PaperlessDocumentSearchDialog from './PaperlessDocumentSearchDialog';
import SeafileFilePickerDialog, { type SeafileLinkTarget } from './SeafileFilePickerDialog';

// The three file-sharing integrations (P2a/P2b/P2c), rendered as rich rows with
// a per-system "Add link" flow. Paperless links documents; Seafile and
// Nextcloud link files/folders.
export type FileSystem = 'paperless' | 'seafile' | 'nextcloud';

interface FileLinksPanelProps {
  identities: ExternalIdentity[];
  configured: { paperless: boolean; seafile: boolean; nextcloud: boolean };
  onFetchPaperlessDocuments?: (query?: string) => Promise<PaperlessDocument[]>;
  onLinkPaperless?: (doc: PaperlessDocument) => Promise<void>;
  onFetchSeafileLibraries?: () => Promise<SeafileLibrary[]>;
  onFetchSeafileDir?: (repoId: string, path: string) => Promise<SeafileItem[]>;
  onLinkSeafile?: (target: SeafileLinkTarget) => Promise<void>;
  onFetchNextcloudDir?: (path?: string) => Promise<WebDAVItem[]>;
  onLinkNextcloud?: (item: WebDAVItem) => Promise<void>;
  onUnlink?: (system: FileSystem, identityId: string) => Promise<void>;
}

// FileLinksPanel is the contact page's "link files/documents" surface for the
// three file-sharing integrations (P2a Paperless-ngx, P2b Seafile, P2c
// Nextcloud/ownCloud WebDAV). Each configured system gets an "Add link"
// affordance and every link of that system renders as a rich row (title/name,
// size, date, deep link, unlink). Links are ExternalIdentity rows on the
// generic T14 substrate.
export default function FileLinksPanel({
  identities,
  configured,
  onFetchPaperlessDocuments,
  onLinkPaperless,
  onFetchSeafileLibraries,
  onFetchSeafileDir,
  onLinkSeafile,
  onFetchNextcloudDir,
  onLinkNextcloud,
  onUnlink,
}: FileLinksPanelProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [error, setError] = useState('');
  const [paperlessOpen, setPaperlessOpen] = useState(false);
  const [seafileOpen, setSeafileOpen] = useState(false);
  const [nextcloudOpen, setNextcloudOpen] = useState(false);

  const systems: { system: FileSystem; icon: string; configured: boolean; label: string }[] = [
    {
      system: 'paperless',
      icon: mdiFileDocumentOutline,
      configured: configured.paperless,
      label: t('fileLinks.paperless'),
    },
    {
      system: 'seafile',
      icon: mdiFolderMultipleOutline,
      configured: configured.seafile,
      label: t('fileLinks.seafile'),
    },
    {
      system: 'nextcloud',
      icon: mdiCloudOutline,
      configured: configured.nextcloud,
      label: t('fileLinks.nextcloud'),
    },
  ];

  const fileIdentities = identities.filter(
    (i) => i.system === 'paperless' || i.system === 'seafile' || i.system === 'nextcloud',
  );

  const handleUnlink = async (system: FileSystem, identityId: string) => {
    if (!onUnlink) return;
    if (!window.confirm(t('fileLinks.unlinkConfirm', { system: systemLabel(system, t) }))) return;
    try {
      await onUnlink(system, identityId);
      setError('');
    } catch (err) {
      setError(err instanceof Error ? err.message : t('fileLinks.unlinkFailed'));
    }
  };

  return (
    <Stack spacing={1.5}>
      {fileIdentities.map((identity) => (
        <Paper key={identity.id} variant="outlined" sx={{ p: 1.5 }}>
          <Box
            sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
              {identity.metadata?.type === 'dir' ? (
                <FolderIcon color="action" fontSize="small" />
              ) : (
                <InsertDriveFileIcon color="action" fontSize="small" />
              )}
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                  {identityName(identity)}
                </Typography>
                <Typography
                  variant="caption"
                  sx={{
                    color: 'text.secondary',
                    overflowWrap: 'anywhere',
                  }}
                >
                  {identitySubtitle(identity, t, formatDate)}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }}>
              {identity.url &&
                (isHttpUrlString(identity.url) ? (
                  <Link
                    href={identity.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label={t('fileLinks.openExternal', {
                      system: systemLabel(identity.system as FileSystem, t),
                    })}
                  >
                    <OpenInNewIcon fontSize="small" />
                  </Link>
                ) : (
                  // A URL that predates the write-time `httpurl` validator is
                  // shown as text, never as an href (T41).
                  <Typography
                    variant="caption"
                    sx={{
                      color: 'text.secondary',
                      maxWidth: 180,
                      overflowWrap: 'anywhere',
                    }}
                  >
                    {identity.url}
                  </Typography>
                ))}
              <IconButton
                size="small"
                color="error"
                title={t('fileLinks.unlink')}
                aria-label={t('fileLinks.unlink')}
                onClick={() => handleUnlink(identity.system as FileSystem, identity.id)}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Box>
          </Box>
        </Paper>
      ))}

      {systems.map((sys) => {
        // Only render the "Add link" affordance when the system is configured
        // AND its browse callbacks are wired (they are, whenever the system is
        // configured — this guard keeps a half-configured panel inert rather
        // than opening a picker that would fetch nothing).
        const browseWired =
          (sys.system === 'paperless' && !!onFetchPaperlessDocuments && !!onLinkPaperless) ||
          (sys.system === 'seafile' &&
            !!onFetchSeafileLibraries &&
            !!onFetchSeafileDir &&
            !!onLinkSeafile) ||
          (sys.system === 'nextcloud' && !!onFetchNextcloudDir && !!onLinkNextcloud);
        return sys.configured && browseWired ? (
          <Button
            key={sys.system}
            size="small"
            variant="outlined"
            startIcon={<AddIcon />}
            onClick={() => {
              if (sys.system === 'paperless') setPaperlessOpen(true);
              if (sys.system === 'seafile') setSeafileOpen(true);
              if (sys.system === 'nextcloud') setNextcloudOpen(true);
            }}
          >
            {t('fileLinks.addLink', { system: sys.label })}
          </Button>
        ) : null;
      })}

      {error && (
        <Alert severity="error" sx={{ py: 0 }}>
          {error}
        </Alert>
      )}

      <PaperlessDocumentSearchDialog
        open={paperlessOpen}
        onClose={() => setPaperlessOpen(false)}
        onFetchDocuments={(query) =>
          onFetchPaperlessDocuments ? onFetchPaperlessDocuments(query) : Promise.resolve([])
        }
        onSelect={(doc) => (onLinkPaperless ? onLinkPaperless(doc) : Promise.resolve())}
      />
      <SeafileFilePickerDialog
        open={seafileOpen}
        onClose={() => setSeafileOpen(false)}
        onFetchLibraries={() =>
          onFetchSeafileLibraries ? onFetchSeafileLibraries() : Promise.resolve([])
        }
        onFetchDir={(repoId, path) =>
          onFetchSeafileDir ? onFetchSeafileDir(repoId, path) : Promise.resolve([])
        }
        onSelect={(target) => (onLinkSeafile ? onLinkSeafile(target) : Promise.resolve())}
      />
      <NextcloudFilePickerDialog
        open={nextcloudOpen}
        onClose={() => setNextcloudOpen(false)}
        onFetchDir={(path) =>
          onFetchNextcloudDir ? onFetchNextcloudDir(path) : Promise.resolve([])
        }
        onSelect={(item) => (onLinkNextcloud ? onLinkNextcloud(item) : Promise.resolve())}
      />
    </Stack>
  );
}

function systemLabel(system: FileSystem, t: TFunction): string {
  switch (system) {
    case 'paperless':
      return t('fileLinks.paperless');
    case 'seafile':
      return t('fileLinks.seafile');
    case 'nextcloud':
      return t('fileLinks.nextcloud');
  }
}

function identityName(identity: ExternalIdentity): string {
  const meta = identity.metadata || {};
  if (typeof meta.title === 'string' && meta.title) return meta.title;
  if (typeof meta.name === 'string' && meta.name) return meta.name;
  return identity.external_id;
}

function identitySubtitle(
  identity: ExternalIdentity,
  t: TFunction,
  formatDate: (date: string) => string,
): string {
  const meta = identity.metadata || {};
  const bits: string[] = [];

  if (identity.system === 'paperless') {
    if (typeof meta.created_at === 'string' && meta.created_at) {
      bits.push(t('fileLinks.created', { date: formatDate(meta.created_at) }));
    }
  } else {
    if (typeof meta.size === 'number' && meta.size > 0) {
      bits.push(formatFileSize(meta.size));
    }
    const modified = typeof meta.modified_at === 'string' ? meta.modified_at : undefined;
    const mtime =
      typeof meta.mtime === 'number' ? new Date(meta.mtime * 1000).toISOString() : undefined;
    const when = modified ?? mtime;
    if (when) {
      bits.push(t('fileLinks.modified', { date: formatDate(when) }));
    }
  }

  bits.push(identity.system);
  return bits.join(' · ');
}

function formatFileSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
