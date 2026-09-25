import { useCallback, useEffect, useState } from 'react';
import {
  getNextcloudConfig,
  linkNextcloudItem,
  unlinkNextcloudItem,
  type WebDAVItem,
} from '../api/nextcloud';
import {
  getPaperlessConfig,
  linkPaperlessDocument,
  type PaperlessDocument,
  unlinkPaperlessDocument,
} from '../api/paperless';
import { getSeafileConfig, linkSeafileItem, unlinkSeafileItem } from '../api/seafile';
import type { SeafileLinkTarget } from '../components/SeafileFilePickerDialog';

export type FileLinkSystem = 'paperless' | 'seafile' | 'nextcloud';

export type FileSystemsConfigured = Record<FileLinkSystem, boolean>;

// File-sharing integrations on the contact page (P2a/P2b/P2c): whether each
// system is configured gates its "Add link" affordance, and link/unlink go
// through the integration endpoints, then re-pull the generic
// ExternalIdentity list via `refreshExternalLinks` so the new row appears.
export function useContactFileLinks(
  contactUid: string | undefined,
  refreshExternalLinks: (uid: string) => Promise<void>,
) {
  // A config fetch failure just leaves that system hidden (the settings page
  // is where connection problems surface), independently per system.
  const [configured, setConfigured] = useState<FileSystemsConfigured>({
    paperless: false,
    seafile: false,
    nextcloud: false,
  });

  useEffect(() => {
    void Promise.allSettled([getPaperlessConfig(), getSeafileConfig(), getNextcloudConfig()]).then(
      ([p, s, n]) => {
        setConfigured({
          paperless: p.status === 'fulfilled' && p.value.has_api_token,
          seafile: s.status === 'fulfilled' && s.value.has_api_token,
          nextcloud: n.status === 'fulfilled' && n.value.has_app_password,
        });
      },
    );
  }, []);

  const handleLinkPaperless = useCallback(
    async (doc: PaperlessDocument) => {
      if (!contactUid) return;
      await linkPaperlessDocument(contactUid, doc.id);
      await refreshExternalLinks(contactUid);
    },
    [contactUid, refreshExternalLinks],
  );

  const handleLinkSeafile = useCallback(
    async (target: SeafileLinkTarget) => {
      if (!contactUid) return;
      await linkSeafileItem(contactUid, {
        repo_id: target.repo_id,
        path: target.path,
        name: target.name,
        type: target.type,
        size: target.size,
        mtime: target.mtime,
      });
      await refreshExternalLinks(contactUid);
    },
    [contactUid, refreshExternalLinks],
  );

  const handleLinkNextcloud = useCallback(
    async (item: WebDAVItem) => {
      if (!contactUid) return;
      await linkNextcloudItem(contactUid, {
        path: item.path,
        name: item.name,
        type: item.type,
        size: item.size,
        modified_at: item.modified_at,
        file_id: item.file_id,
      });
      await refreshExternalLinks(contactUid);
    },
    [contactUid, refreshExternalLinks],
  );

  const handleUnlink = useCallback(
    async (system: FileLinkSystem, identityId: string) => {
      if (!contactUid) return;
      if (system === 'paperless') await unlinkPaperlessDocument(contactUid, identityId);
      if (system === 'seafile') await unlinkSeafileItem(contactUid, identityId);
      if (system === 'nextcloud') await unlinkNextcloudItem(contactUid, identityId);
      await refreshExternalLinks(contactUid);
    },
    [contactUid, refreshExternalLinks],
  );

  return {
    configured,
    handleLinkPaperless,
    handleLinkSeafile,
    handleLinkNextcloud,
    handleUnlink,
  };
}
