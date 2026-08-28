import { useCallback, useRef, useState } from 'react';
import { type DiagnosticsResponse, runDiagnostics } from '../api/diagnostics';
import { handleFetchError } from '../utils/errorHandler';

// useDiagnostics drives the admin "Run diagnostics" panel (issue #423). Unlike
// the continuously-derived health panels, the sweep is a manual action, so it
// starts empty and only runs when the operator clicks "Run diagnostics"; the
// previous result stays visible while a fresh run is in flight.
export function useDiagnostics() {
  const [data, setData] = useState<DiagnosticsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const run = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);
    try {
      const resp = await runDiagnostics();
      if (requestRef.current !== requestId) return;
      setData(resp);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'running diagnostics'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, []);

  return { data, loading, error, run };
}
