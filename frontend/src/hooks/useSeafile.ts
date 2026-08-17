import { useState, useCallback } from 'react';
import {
  getSeafileConfig,
  saveSeafileConfig,
  deleteSeafileConfig,
  getSeafileLibraries,
  getSeafileDir,
  testSeafileConnection,
  SeafileConfigResponse,
  SeafileConnectionTestResult,
  SeafileLibrary,
  SeafileItem,
} from '../api/seafile';
import { handleFetchError, handleError, ErrorNotifier } from '../utils/errorHandler';

// useSeafile backs both the settings connection card and the contact-page
// Seafile link surface (P2b). Connection config is per-user-global; per
// contact there can be many linked files/folders (each an ExternalIdentity).
export function useSeafile(notifier?: ErrorNotifier) {
  const [config, setConfig] = useState<SeafileConfigResponse | null>(null);
  const [configLoading, setConfigLoading] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);

  const [libraries, setLibraries] = useState<SeafileLibrary[]>([]);
  const [items, setItems] = useState<SeafileItem[]>([]);
  const [browsing, setBrowsing] = useState(false);

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SeafileConnectionTestResult | null>(null);

  const refreshConfig = useCallback(async () => {
    setConfigLoading(true);
    setConfigError(null);
    try {
      setConfig(await getSeafileConfig());
    } catch (err) {
      setConfigError(handleFetchError(err, 'fetching Seafile config'));
    } finally {
      setConfigLoading(false);
    }
  }, []);

  const saveConfig = useCallback(
    async (input: { base_url: string; api_token?: string }) => {
      try {
        const updated = await saveSeafileConfig(input);
        setConfig(updated);
        return updated;
      } catch (err) {
        handleError(err, { operation: 'saving Seafile connection' }, notifier);
        throw err;
      }
    },
    [notifier]
  );

  const removeConfig = useCallback(async () => {
    await deleteSeafileConfig();
    setConfig(null);
  }, []);

  const browseLibraries = useCallback(async () => {
    setBrowsing(true);
    try {
      const fetched = await getSeafileLibraries();
      setLibraries(fetched);
      return fetched;
    } catch (err) {
      handleError(err, { operation: 'browsing Seafile libraries' }, notifier);
      throw err;
    } finally {
      setBrowsing(false);
    }
  }, [notifier]);

  const browseDir = useCallback(
    async (repoId: string, path: string) => {
      setBrowsing(true);
      try {
        const fetched = await getSeafileDir(repoId, path);
        setItems(fetched);
        return fetched;
      } catch (err) {
        handleError(err, { operation: 'browsing Seafile folder' }, notifier);
        throw err;
      } finally {
        setBrowsing(false);
      }
    },
    [notifier]
  );

  const testConnection = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testSeafileConnection();
      setTestResult(result);
      return result;
    } catch (err) {
      handleError(err, { operation: 'testing Seafile connection' }, notifier);
      throw err;
    } finally {
      setTesting(false);
    }
  }, [notifier]);

  return {
    config,
    configLoading,
    configError,
    libraries,
    items,
    browsing,
    testing,
    testResult,
    refreshConfig,
    saveConfig,
    removeConfig,
    browseLibraries,
    browseDir,
    testConnection,
  };
}
