import { useCallback, useRef, useState } from 'react';
import {
  deleteGeoPulseConfig,
  type GeoPulseConfigInput,
  type GeoPulseConfigResponse,
  type GeoPulseConnectionTestResult,
  type GeoPulseSuggestionsResponse,
  getGeoPulseConfig,
  getGeoPulseSuggestions,
  saveGeoPulseConfig,
  testGeoPulseConnection,
} from '../api/geopulse';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

// useGeoPulse backs both the settings connection card and the "log activity from
// location history" dialog (issue #160). Suggestions are ephemeral component
// state — nothing about them is cached or persisted client-side either.
export function useGeoPulse(notifier?: ErrorNotifier) {
  const [config, setConfig] = useState<GeoPulseConfigResponse | null>(null);
  const [configLoading, setConfigLoading] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<GeoPulseConnectionTestResult | null>(null);

  const [suggestions, setSuggestions] = useState<GeoPulseSuggestionsResponse | null>(null);
  const [suggestionsLoading, setSuggestionsLoading] = useState(false);
  const [suggestionsError, setSuggestionsError] = useState<string | null>(null);

  // Latest-request guard: picking a second date while the first lookup is still
  // in flight must not let the slower, older response overwrite the newer one.
  const lookupSeq = useRef(0);

  const refreshConfig = useCallback(async () => {
    setConfigLoading(true);
    setConfigError(null);
    try {
      setConfig(await getGeoPulseConfig());
    } catch (err) {
      setConfigError(handleFetchError(err, 'fetching GeoPulse config'));
    } finally {
      setConfigLoading(false);
    }
  }, []);

  const saveConfig = useCallback(
    async (input: GeoPulseConfigInput) => {
      try {
        const updated = await saveGeoPulseConfig(input);
        setConfig(updated);
        return updated;
      } catch (err) {
        handleError(err, { operation: 'saving GeoPulse connection' }, notifier);
        throw err;
      }
    },
    [notifier],
  );

  const removeConfig = useCallback(async () => {
    await deleteGeoPulseConfig();
    setConfig(null);
    setTestResult(null);
  }, []);

  const testConnection = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testGeoPulseConnection();
      setTestResult(result);
      return result;
    } catch (err) {
      handleError(err, { operation: 'testing GeoPulse connection' }, notifier);
      throw err;
    } finally {
      setTesting(false);
    }
  }, [notifier]);

  // A failed lookup is shown inline by the dialog (setSuggestionsError), not
  // toasted — the user is looking right at it and needs to see the reason
  // (not configured, bad token, unreachable) next to the date they picked.
  const lookup = useCallback(async (date: string, timezone?: string) => {
    const seq = ++lookupSeq.current;
    setSuggestionsLoading(true);
    setSuggestionsError(null);
    setSuggestions(null);
    try {
      const result = await getGeoPulseSuggestions(date, timezone);
      if (seq === lookupSeq.current) setSuggestions(result);
      return result;
    } catch (err) {
      const message = handleFetchError(err, 'looking up GeoPulse location history');
      if (seq === lookupSeq.current) setSuggestionsError(message);
      return null;
    } finally {
      if (seq === lookupSeq.current) setSuggestionsLoading(false);
    }
  }, []);

  const clearSuggestions = useCallback(() => {
    lookupSeq.current++; // invalidate any lookup still in flight
    setSuggestions(null);
    setSuggestionsError(null);
    setSuggestionsLoading(false);
  }, []);

  return {
    config,
    configLoading,
    configError,
    refreshConfig,
    saveConfig,
    removeConfig,
    testing,
    testResult,
    testConnection,
    suggestions,
    suggestionsLoading,
    suggestionsError,
    lookup,
    clearSuggestions,
  };
}
