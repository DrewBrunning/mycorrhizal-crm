import { useCallback, useState } from 'react';
import { type Contact, getContactsByUid } from '../api/contacts';
import {
  type Feed,
  type FeedCreateResponse,
  listFeeds,
  revokeAllFeeds,
  revokeFeed,
  rotateFeed,
} from '../api/feeds';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

/** Display name for a resolved contact; empty when it has none. */
export function feedContactName(contact: Contact): string {
  return [contact.firstname, contact.lastname].filter(Boolean).join(' ') || contact.nickname || '';
}

export function useFeeds(notifier?: ErrorNotifier) {
  const [feeds, setFeeds] = useState<Feed[]>([]);
  // Contact.VCardUID -> display name, for `contact` feeds' "kind" column.
  const [contactNames, setContactNames] = useState<Map<string, string>>(new Map());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const fetched = await listFeeds();
      setFeeds(fetched);
      const uids = fetched.filter((f) => f.kind === 'contact').map((f) => f.entity_id);
      const contacts = uids.length > 0 ? await getContactsByUid(uids) : new Map<string, Contact>();
      setContactNames(new Map([...contacts].map(([uid, c]) => [uid, feedContactName(c)])));
    } catch (err) {
      setError(handleFetchError(err, 'fetching feeds'));
    } finally {
      setLoading(false);
    }
  }, []);

  // Rotate returns the replacement's one-time URL; the caller shows it.
  const handleRotate = async (id: string): Promise<FeedCreateResponse> => {
    try {
      const result = await rotateFeed(id);
      await refresh();
      return result;
    } catch (err) {
      handleError(err, { operation: 'rotating feed' }, notifier);
      throw err;
    }
  };

  const handleRevoke = async (id: string): Promise<void> => {
    try {
      await revokeFeed(id);
      await refresh();
    } catch (err) {
      handleError(err, { operation: 'revoking feed' }, notifier);
      throw err;
    }
  };

  const handleRevokeAll = async (): Promise<number> => {
    try {
      const { revoked } = await revokeAllFeeds();
      await refresh();
      return revoked;
    } catch (err) {
      handleError(err, { operation: 'revoking all feeds' }, notifier);
      throw err;
    }
  };

  return {
    feeds,
    contactNames,
    loading,
    error,
    refresh,
    handleRotate,
    handleRevoke,
    handleRevokeAll,
  };
}
