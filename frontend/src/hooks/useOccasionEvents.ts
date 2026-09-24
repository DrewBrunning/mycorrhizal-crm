import { useCallback, useEffect, useState } from 'react';
import {
  addOccasionEventAttendee,
  createOccasionEvent,
  deleteOccasionEvent,
  getInviteeSuggestions,
  getOccasionEvent,
  getOccasionEvents,
  type InviteeSuggestion,
  type OccasionEvent,
  type OccasionEventAttendeeView,
  type OccasionEventInput,
  type OccasionEventRSVP,
  removeOccasionEventAttendee,
  updateOccasionEvent,
  updateOccasionEventAttendee,
} from '../api/occasionEvents';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

// useOccasionEvents loads and manages the user's one-off events
// (docs/adrs/0026-occasions-events.md, issue #1228). Mirrors
// useOccasionObligations' exact shape.
export function useOccasionEvents(notifier?: ErrorNotifier) {
  const [events, setEvents] = useState<OccasionEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await getOccasionEvents({ limit: 100 });
      setEvents(response.occasion_events || []);
    } catch (err) {
      setError(handleFetchError(err, 'fetching occasion events'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleSave = useCallback(
    async (event: OccasionEvent | null, input: OccasionEventInput) => {
      try {
        if (event) {
          await updateOccasionEvent(event.id, input);
        } else {
          await createOccasionEvent(input);
        }
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'saving occasion event' }, notifier);
        throw err;
      }
    },
    [refresh, notifier],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await deleteOccasionEvent(id);
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'deleting occasion event' }, notifier);
        throw err;
      }
    },
    [refresh, notifier],
  );

  return { events, loading, error, refresh, handleSave, handleDelete };
}

// useOccasionEventAttendees loads one event's attendee/RSVP list and manages
// it (add / set RSVP / remove), plus circle-based invitee suggestions. The
// RSVP it records is what the user reports the contact told them — nothing is
// sent.
export function useOccasionEventAttendees(eventId: string | undefined, notifier?: ErrorNotifier) {
  const [attendees, setAttendees] = useState<OccasionEventAttendeeView[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [suggestions, setSuggestions] = useState<InviteeSuggestion[]>([]);
  const [suggestionsLoading, setSuggestionsLoading] = useState(false);

  const refresh = useCallback(async () => {
    if (!eventId) {
      setAttendees([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const detail = await getOccasionEvent(eventId);
      setAttendees(detail.attendees || []);
    } catch (err) {
      setError(handleFetchError(err, 'fetching event attendees'));
    } finally {
      setLoading(false);
    }
  }, [eventId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleAdd = useCallback(
    async (entityId: string) => {
      if (!eventId) return;
      try {
        await addOccasionEventAttendee(eventId, { entity_id: entityId });
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'adding event attendee' }, notifier);
        throw err;
      }
    },
    [eventId, refresh, notifier],
  );

  const handleUpdateRsvp = useCallback(
    async (vcardUid: string, rsvp: OccasionEventRSVP) => {
      if (!eventId) return;
      try {
        await updateOccasionEventAttendee(eventId, vcardUid, rsvp);
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'updating attendee RSVP' }, notifier);
        throw err;
      }
    },
    [eventId, refresh, notifier],
  );

  const handleRemove = useCallback(
    async (vcardUid: string) => {
      if (!eventId) return;
      try {
        await removeOccasionEventAttendee(eventId, vcardUid);
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'removing event attendee' }, notifier);
        throw err;
      }
    },
    [eventId, refresh, notifier],
  );

  const loadSuggestions = useCallback(
    async (circleIds: string[]) => {
      if (!eventId || circleIds.length === 0) {
        setSuggestions([]);
        return;
      }
      setSuggestionsLoading(true);
      try {
        setSuggestions(await getInviteeSuggestions({ circleIds, eventId }));
      } catch (err) {
        handleError(err, { operation: 'loading invitee suggestions' }, notifier);
        setSuggestions([]);
      } finally {
        setSuggestionsLoading(false);
      }
    },
    [eventId, notifier],
  );

  const clearSuggestions = useCallback(() => setSuggestions([]), []);

  return {
    attendees,
    loading,
    error,
    refresh,
    handleAdd,
    handleUpdateRsvp,
    handleRemove,
    suggestions,
    suggestionsLoading,
    loadSuggestions,
    clearSuggestions,
  };
}
