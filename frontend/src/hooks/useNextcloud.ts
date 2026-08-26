import { useCallback, useState } from 'react';
import {
  deleteNextcloudConfig,
  getNextcloudConfig,
  getNextcloudDir,
  saveNextcloudConfig,
  testNextcloudConnection,
  type WebDAVConfigResponse,
  type WebDAVConnectionTestResult,
  type WebDAVItem,
} from '../api/nextcloud';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

// useNextcloud backs both the settings connection card and the contact-page
// Nextcloud/ownCloud (WebDAV) link surface (P2c). Connection config is
// per-user-global; per contact there can be many linked files/folders (each an
// ExternalIdentity).
export function useNextcloud(notifier?: ErrorNotifier) {
  const [config, setConfig] = useState<WebDAVConfigResponse | null>(null);
  const [configLoading, setConfigLoading] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);

  const [items, setItems] = useState<WebDAVItem[]>([]);
  const [browsing, setBrowsing] = useState(false);

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<WebDAVConnectionTestResult | null>(null);

  const refreshConfig = useCallback(async () => {
    setConfigLoading(true);
    setConfigError(null);
    try {
      setConfig(await getNextcloudConfig());
    } catch (err) {
      setConfigError(handleFetchError(err, 'fetching Nextcloud config'));
    } finally {
      setConfigLoading(false);
    }
  }, []);

  const saveConfig = useCallback(
    async (input: { base_url: string; username: string; app_password?: string }) => {
      try {
        const updated = await saveNextcloudConfig(input);
        setConfig(updated);
        return updated;
      } catch (err) {
        handleError(err, { operation: 'saving Nextcloud connection' }, notifier);
        throw err;
      }
    },
    [notifier],
  );

  const removeConfig = useCallback(async () => {
    await deleteNextcloudConfig();
    setConfig(null);
  }, []);

  const browseDir = useCallback(
    async (path?: string) => {
      setBrowsing(true);
      try {
        const fetched = await getNextcloudDir(path);
        setItems(fetched);
        return fetched;
      } catch (err) {
        handleError(err, { operation: 'browsing Nextcloud folder' }, notifier);
        throw err;
      } finally {
        setBrowsing(false);
      }
    },
    [notifier],
  );

  const testConnection = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testNextcloudConnection();
      setTestResult(result);
      return result;
    } catch (err) {
      handleError(err, { operation: 'testing Nextcloud connection' }, notifier);
      throw err;
    } finally {
      setTesting(false);
    }
  }, [notifier]);

  return {
    config,
    configLoading,
    configError,
    items,
    browsing,
    testing,
    testResult,
    refreshConfig,
    saveConfig,
    removeConfig,
    browseDir,
    testConnection,
  };
}
