import { useState, useCallback } from 'react';
import {
  getImmichConfig,
  saveImmichConfig,
  deleteImmichConfig,
  getImmichPeople,
  getImmichContactSummary,
  linkImmichPerson,
  unlinkImmichPerson,
  syncImmich,
  ImmichConfigResponse,
  ImmichPerson,
  ImmichPersonSummary,
} from '../api/immich';
import { handleFetchError, handleError, ErrorNotifier } from '../utils/errorHandler';

// useImmich backs both the settings connection card and the contact-page
// Immich link surface (T15/T16). Connection config is per-user-global; per
// contact there is at most one linked person (the natural "one contact ↔ one
// Immich person" relationship, stored as an ExternalIdentity).
export function useImmich(notifier?: ErrorNotifier) {
  const [config, setConfig] = useState<ImmichConfigResponse | null>(null);
  const [configLoading, setConfigLoading] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);

  const [summary, setSummary] = useState<ImmichPersonSummary | null>(null);
  const [summaryLoading, setSummaryLoading] = useState(false);

  const [people, setPeople] = useState<ImmichPerson[]>([]);
  const [peopleLoading, setPeopleLoading] = useState(false);

  const [syncing, setSyncing] = useState(false);

  const refreshConfig = useCallback(async () => {
    setConfigLoading(true);
    setConfigError(null);
    try {
      setConfig(await getImmichConfig());
    } catch (err) {
      setConfigError(handleFetchError(err, 'fetching Immich config'));
    } finally {
      setConfigLoading(false);
    }
  }, []);

  const saveConfig = useCallback(
    async (input: { base_url: string; api_key?: string; sync_enabled?: boolean }) => {
      try {
        const updated = await saveImmichConfig(input);
        setConfig(updated);
        return updated;
      } catch (err) {
        handleError(err, { operation: 'saving Immich connection' }, notifier);
        throw err;
      }
    },
    [notifier]
  );

  const removeConfig = useCallback(async () => {
    await deleteImmichConfig();
    setConfig(null);
  }, []);

  const browsePeople = useCallback(async () => {
    setPeopleLoading(true);
    try {
      const fetched = await getImmichPeople();
      setPeople(fetched);
      return fetched;
    } catch (err) {
      handleError(err, { operation: 'browsing Immich people' }, notifier);
      throw err;
    } finally {
      setPeopleLoading(false);
    }
  }, [notifier]);

  const loadSummary = useCallback(async (contactUid: string) => {
    setSummaryLoading(true);
    try {
      const fetched = await getImmichContactSummary(contactUid);
      setSummary(fetched);
      return fetched;
    } catch {
      return null;
    } finally {
      setSummaryLoading(false);
    }
  }, []);

  const linkPerson = useCallback(
    async (contactUid: string, personId: string, personName: string) => {
      await linkImmichPerson(contactUid, personId, personName);
    },
    []
  );

  const unlinkPerson = useCallback(async (contactUid: string) => {
    await unlinkImmichPerson(contactUid);
    setSummary(null);
  }, []);

  const runSync = useCallback(async () => {
    setSyncing(true);
    try {
      await syncImmich();
      await refreshConfig();
    } catch (err) {
      handleError(err, { operation: 'syncing Immich' }, notifier);
      throw err;
    } finally {
      setSyncing(false);
    }
  }, [refreshConfig, notifier]);

  return {
    config,
    configLoading,
    configError,
    summary,
    summaryLoading,
    people,
    peopleLoading,
    syncing,
    refreshConfig,
    saveConfig,
    removeConfig,
    browsePeople,
    loadSummary,
    linkPerson,
    unlinkPerson,
    runSync,
  };
}
