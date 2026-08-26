import { useCallback, useState } from 'react';
import {
  deletePaperlessConfig,
  getPaperlessConfig,
  getPaperlessDocuments,
  type PaperlessConfigResponse,
  type PaperlessConnectionTestResult,
  type PaperlessDocument,
  savePaperlessConfig,
  testPaperlessConnection,
} from '../api/paperless';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

// usePaperless backs both the settings connection card and the contact-page
// Paperless link surface (P2a). Connection config is per-user-global; per
// contact there can be many linked documents (each an ExternalIdentity).
export function usePaperless(notifier?: ErrorNotifier) {
  const [config, setConfig] = useState<PaperlessConfigResponse | null>(null);
  const [configLoading, setConfigLoading] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);

  const [documents, setDocuments] = useState<PaperlessDocument[]>([]);
  const [documentsLoading, setDocumentsLoading] = useState(false);

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<PaperlessConnectionTestResult | null>(null);

  const refreshConfig = useCallback(async () => {
    setConfigLoading(true);
    setConfigError(null);
    try {
      setConfig(await getPaperlessConfig());
    } catch (err) {
      setConfigError(handleFetchError(err, 'fetching Paperless config'));
    } finally {
      setConfigLoading(false);
    }
  }, []);

  const saveConfig = useCallback(
    async (input: { base_url: string; api_token?: string }) => {
      try {
        const updated = await savePaperlessConfig(input);
        setConfig(updated);
        return updated;
      } catch (err) {
        handleError(err, { operation: 'saving Paperless connection' }, notifier);
        throw err;
      }
    },
    [notifier],
  );

  const removeConfig = useCallback(async () => {
    await deletePaperlessConfig();
    setConfig(null);
  }, []);

  const browseDocuments = useCallback(
    async (query?: string) => {
      setDocumentsLoading(true);
      try {
        const fetched = await getPaperlessDocuments(query);
        setDocuments(fetched);
        return fetched;
      } catch (err) {
        handleError(err, { operation: 'browsing Paperless documents' }, notifier);
        throw err;
      } finally {
        setDocumentsLoading(false);
      }
    },
    [notifier],
  );

  const testConnection = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testPaperlessConnection();
      setTestResult(result);
      return result;
    } catch (err) {
      handleError(err, { operation: 'testing Paperless connection' }, notifier);
      throw err;
    } finally {
      setTesting(false);
    }
  }, [notifier]);

  return {
    config,
    configLoading,
    configError,
    documents,
    documentsLoading,
    testing,
    testResult,
    refreshConfig,
    saveConfig,
    removeConfig,
    browseDocuments,
    testConnection,
  };
}
