import { useState, useCallback, useEffect, useRef } from 'react';
import { getAuditEvents, undoAuditEvent, AuditEvent, AuditEntityType } from '../api/audit';
import { handleFetchError } from '../utils/errorHandler';

const DEFAULT_LIMIT = 100;
const MAX_LIMIT = 500;

// useAudit drives the audit-log page (T60). Filtering is server-side by
// entity_type/entity_id (the API supports it; filtering client-side would drop
// the offset of the limit window). `limit` starts at the API's 100 default and
// grows in 100-event steps up to the 500 cap when the list fills the window.
export function useAudit() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [entityType, setEntityType] = useState<AuditEntityType | ''>('');
  const [entityId, setEntityId] = useState('');
  const [limit, setLimit] = useState(DEFAULT_LIMIT);
  // Guards against out-of-order responses when a filter change races an
  // in-flight fetch (same pattern as useActivities).
  const requestRef = useRef(0);

  const fetchEvents = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);

    try {
      const data = await getAuditEvents({
        entity_type: entityType || undefined,
        entity_id: entityId.trim() || undefined,
        limit,
      });
      if (requestRef.current !== requestId) return;
      setEvents(data.audit_events || []);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching audit events'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [entityType, entityId, limit]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  // A changed filter restarts the window at the default limit -- the pageless
  // API only returns the newest `limit` rows, so a previous Load-more window
  // must not silently mask the filtered result.
  const applyEntityType = useCallback((type: AuditEntityType | '') => {
    setEntityType(type);
    setLimit(DEFAULT_LIMIT);
  }, []);

  const applyEntityId = useCallback((id: string) => {
    setEntityId(id);
    setLimit(DEFAULT_LIMIT);
  }, []);

  const clearFilters = useCallback(() => {
    setEntityType('');
    setEntityId('');
    setLimit(DEFAULT_LIMIT);
  }, []);

  // The API returns `total` = the number of rows in this window (no cursor),
  // so "a full window means there might be more" is the only signal available.
  const canLoadMore = events.length >= limit && limit < MAX_LIMIT;

  const loadMore = useCallback(() => {
    if (!canLoadMore) return;
    setLimit((prev) => Math.min(prev + DEFAULT_LIMIT, MAX_LIMIT));
  }, [canLoadMore]);

  // Reverts an update event via POST /audit/:id/undo, then refreshes so the
  // list reflects the restored state. Errors (400 unsupported/delete, 410 past
  // retention, 404 gone) propagate to the caller so the page can surface the
  // exact failure to the user.
  const handleUndo = useCallback(
    async (id: number) => {
      await undoAuditEvent(id);
      await fetchEvents();
    },
    [fetchEvents]
  );

  return {
    events,
    loading,
    error,
    entityType,
    applyEntityType,
    applyEntityId,
    clearFilters,
    refetch: fetchEvents,
    handleUndo,
    canLoadMore,
    loadMore,
  };
}
