import { useCallback, useState } from 'react';
import {
  type ExternalActivity,
  type ExternalIdentity,
  getExternalActivities,
  getExternalIdentities,
} from '../api/externalLinks';
import { handleFetchError } from '../utils/errorHandler';

// useExternalLinks backs the contact page's external-link surface (T14): the
// identities ("this contact IS this thing in that system") and enrichment
// activities ("something that happened in an external system", linkable into
// the timeline) for one contact, keyed by VCardUID.
export function useExternalLinks(contactUid: string | undefined) {
  const [identities, setIdentities] = useState<ExternalIdentity[]>([]);
  const [activities, setActivities] = useState<ExternalActivity[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(
    async (overrideUid?: string) => {
      const uid = overrideUid ?? contactUid;
      if (!uid) return;
      setLoading(true);
      setError(null);
      try {
        const [identitiesRes, activitiesRes] = await Promise.all([
          getExternalIdentities({ contactId: uid, limit: 100 }),
          getExternalActivities({ contactId: uid, limit: 100 }),
        ]);
        setIdentities(identitiesRes.external_identities || []);
        setActivities(activitiesRes.external_activities || []);
      } catch (err) {
        setError(handleFetchError(err, 'fetching external links'));
      } finally {
        setLoading(false);
      }
    },
    [contactUid],
  );

  return {
    identities,
    activities,
    loading,
    error,
    refresh,
  };
}
