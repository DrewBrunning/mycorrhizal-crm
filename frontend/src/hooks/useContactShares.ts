import { useState, useCallback } from 'react';
import {
  getIncomingContactShares,
  getOutgoingContactShares,
  acceptContactShare,
  confirmContactShare,
  declineContactShare,
  ContactShare,
} from '../api/contactShares';
import { ImportPreviewResponse, RowImportAction } from '../api/import';
import { handleFetchError, handleError, ErrorNotifier } from '../utils/errorHandler';

export function useContactShares(notifier?: ErrorNotifier) {
  const [incoming, setIncoming] = useState<ContactShare[]>([]);
  const [outgoing, setOutgoing] = useState<ContactShare[]>([]);
  const [usernames, setUsernames] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [incomingResp, outgoingResp] = await Promise.all([
        getIncomingContactShares(),
        getOutgoingContactShares(),
      ]);
      setIncoming(incomingResp.contact_shares || []);
      setOutgoing(outgoingResp.contact_shares || []);
      setUsernames({ ...outgoingResp.usernames, ...incomingResp.usernames });
    } catch (err) {
      setError(handleFetchError(err, 'fetching contact shares'));
    } finally {
      setLoading(false);
    }
  }, []);

  // Preview-only step: parses the share's payload and returns what
  // handleConfirmShare needs. Does not change the share's status.
  const handleAcceptPreview = async (shareId: string): Promise<ImportPreviewResponse | undefined> => {
    try {
      return await acceptContactShare(shareId);
    } catch (err) {
      handleError(err, { operation: 'previewing contact share' }, notifier);
      throw err;
    }
  };

  const handleConfirmShare = async (shareId: string, sessionId: string, actions: RowImportAction[]) => {
    try {
      const result = await confirmContactShare(shareId, sessionId, actions);
      await refresh();
      return result;
    } catch (err) {
      handleError(err, { operation: 'confirming contact share' }, notifier);
      throw err;
    }
  };

  const handleDeclineShare = async (shareId: string) => {
    try {
      await declineContactShare(shareId);
      await refresh();
    } catch (err) {
      handleError(err, { operation: 'declining contact share' }, notifier);
      throw err;
    }
  };

  return {
    incoming,
    outgoing,
    usernames,
    loading,
    error,
    refresh,
    handleAcceptPreview,
    handleConfirmShare,
    handleDeclineShare,
  };
}
