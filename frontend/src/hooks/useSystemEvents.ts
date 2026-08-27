import { useCallback, useEffect, useRef, useState } from 'react';
import {
  type GetSystemEventsParams,
  getSystemEvents,
  type SystemEvent,
  type SystemEventSeverity,
  type SystemEventType,
} from '../api/systemEvents';
import { handleFetchError } from '../utils/errorHandler';

const DEFAULT_LIMIT = 100;
const MAX_LIMIT = 500;

export interface SystemEventFilters {
  component: string;
  severity: SystemEventSeverity | '';
  eventType: SystemEventType | '';
  correlationId: string;
}

const EMPTY_FILTERS: SystemEventFilters = {
  component: '',
  severity: '',
  eventType: '',
  correlationId: '',
};

// useSystemEvents drives the operational-event timeline (issue #424).
// Filtering is server-side (the pageless API returns only the newest `limit`
// rows, so client-side filtering would silently drop matches outside the
// window). `limit` starts at the API default and grows in steps up to the
// 500 cap, mirroring useAudit.
export function useSystemEvents() {
  const [events, setEvents] = useState<SystemEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState<SystemEventFilters>(EMPTY_FILTERS);
  const [limit, setLimit] = useState(DEFAULT_LIMIT);
  const requestRef = useRef(0);

  const fetchEvents = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);

    const params: GetSystemEventsParams = {
      component: filters.component || undefined,
      severity: filters.severity || undefined,
      event_type: filters.eventType || undefined,
      correlation_id: filters.correlationId.trim() || undefined,
      limit,
    };

    try {
      const data = await getSystemEvents(params);
      if (requestRef.current !== requestId) return;
      setEvents(data.system_events || []);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching system events'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [filters, limit]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  // Any filter change restarts the window at the default limit.
  const patchFilters = useCallback((patch: Partial<SystemEventFilters>) => {
    setFilters((prev) => ({ ...prev, ...patch }));
    setLimit(DEFAULT_LIMIT);
  }, []);

  const clearFilters = useCallback(() => {
    setFilters(EMPTY_FILTERS);
    setLimit(DEFAULT_LIMIT);
  }, []);

  // Jump to every event sharing one correlation ID — the timeline's
  // "view related" drill-down. Clears the other filters so the chain is shown
  // whole.
  const showRelated = useCallback((correlationId: string) => {
    setFilters({ ...EMPTY_FILTERS, correlationId });
    setLimit(MAX_LIMIT);
  }, []);

  const hasFilters =
    filters.component !== '' ||
    filters.severity !== '' ||
    filters.eventType !== '' ||
    filters.correlationId.trim() !== '';

  const canLoadMore = events.length >= limit && limit < MAX_LIMIT;
  const loadMore = useCallback(() => {
    setLimit((prev) => Math.min(prev + DEFAULT_LIMIT, MAX_LIMIT));
  }, []);

  return {
    events,
    loading,
    error,
    filters,
    hasFilters,
    patchFilters,
    clearFilters,
    showRelated,
    refetch: fetchEvents,
    canLoadMore,
    loadMore,
  };
}
