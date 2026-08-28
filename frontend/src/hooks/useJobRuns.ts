import { useCallback, useEffect, useRef, useState } from 'react';
import {
  getJobRunHealth,
  type JobRun,
  type JobRunHealth,
  type ListJobRunsParams,
  listJobRuns,
} from '../api/jobRuns';
import { handleFetchError } from '../utils/errorHandler';

// useJobRunHealth drives the background-job monitor panel (issue #391).
// Load-once with an explicit refresh — the state is a server-side projection
// with no filters and no pagination, mirroring useSubsystemHealth.
export function useJobRunHealth() {
  const [data, setData] = useState<JobRunHealth[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);
    try {
      const resp = await getJobRunHealth();
      if (requestRef.current !== requestId) return;
      setData(resp.jobs || []);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching background job health'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { data, loading, error, refresh };
}

// useJobRunHistory lazily loads the per-job run history for the drill-down —
// nothing is fetched until load() is called with a job name.
export function useJobRunHistory() {
  const [data, setData] = useState<JobRun[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const load = useCallback(async (params: ListJobRunsParams) => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);
    try {
      const resp = await listJobRuns(params);
      if (requestRef.current !== requestId) return;
      setData(resp.job_runs || []);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching job run history'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, []);

  const reset = useCallback(() => {
    requestRef.current += 1;
    setData([]);
    setError(null);
    setLoading(false);
  }, []);

  return { data, loading, error, load, reset };
}
