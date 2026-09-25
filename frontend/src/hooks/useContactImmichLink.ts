import { useCallback, useEffect, useState } from 'react';
import {
  getImmichConfig,
  getImmichContactSummary,
  type ImmichPerson,
  type ImmichPersonSummary,
  linkImmichPerson,
  syncImmich,
  unlinkImmichPerson,
} from '../api/immich';

// The contact page's Immich surface (T15/T16): whether Immich is configured
// at all (gates the "Choose from Immich" profile-photo entry point), this
// contact's linked-person summary, and link/unlink/sync. Every mutation
// re-pulls the generic ExternalIdentity list via `refreshExternalLinks` so the
// external-links panel stays in step.
export function useContactImmichLink(
  contactUid: string | undefined,
  refreshExternalLinks: (uid: string) => Promise<void>,
) {
  const [summary, setSummary] = useState<ImmichPersonSummary | null>(null);
  const [summaryLoading, setSummaryLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  // Failure just leaves the Immich entry points hidden -- the settings page
  // is where connection problems surface.
  const [configured, setConfigured] = useState(false);

  useEffect(() => {
    getImmichConfig()
      .then((cfg) => setConfigured(cfg.has_api_key))
      .catch(() => setConfigured(false));
  }, []);

  // Accepts an override uid for the page's first load, before contactUid has
  // re-rendered through state. A failed fetch reads as "not linked".
  const refreshSummary = useCallback(
    async (overrideUid?: string) => {
      const uid = overrideUid ?? contactUid;
      if (!uid) return;
      setSummaryLoading(true);
      try {
        setSummary(await getImmichContactSummary(uid));
      } catch {
        setSummary(null);
      } finally {
        setSummaryLoading(false);
      }
    },
    [contactUid],
  );

  const handleLink = useCallback(
    async (person: ImmichPerson) => {
      if (!contactUid) return;
      await linkImmichPerson(contactUid, person.id, person.name);
      await Promise.all([refreshExternalLinks(contactUid), refreshSummary(contactUid)]);
    },
    [contactUid, refreshExternalLinks, refreshSummary],
  );

  const handleUnlink = useCallback(async () => {
    if (!contactUid) return;
    await unlinkImmichPerson(contactUid);
    setSummary(null);
    await refreshExternalLinks(contactUid);
  }, [contactUid, refreshExternalLinks]);

  const handleSync = useCallback(async () => {
    if (!contactUid) return;
    setSyncing(true);
    try {
      await syncImmich();
      await Promise.all([refreshExternalLinks(contactUid), refreshSummary(contactUid)]);
    } finally {
      setSyncing(false);
    }
  }, [contactUid, refreshExternalLinks, refreshSummary]);

  return {
    configured,
    summary,
    summaryLoading,
    syncing,
    refreshSummary,
    handleLink,
    handleUnlink,
    handleSync,
  };
}
