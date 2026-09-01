import { useCallback, useEffect, useRef, useState } from 'react';
import {
  cancelSourceImport,
  confirmSourceImport,
  getSourceImportPreview,
  getSourceImportStatus,
  type RowSourceAction,
  type SourceImportPreviewResponse,
  type SourceImportResult,
  type SourceImportStatus,
} from '../api/sourceImport';
import { getErrorMessage } from '../utils/errorHandler';

// The five wizard steps, shared by every source-import assistant. The
// source-specific "connect" UI owns step 0 and hands us a session id via
// beginFetch; everything after is generic.
export type SourceImportStep = 'connect' | 'fetching' | 'review' | 'importing' | 'result';

const POLL_INTERVAL_MS = 1500;

export interface UseSourceImportWizardOptions {
  // API path prefix for the shared status/preview/confirm/cancel endpoints,
  // e.g. "/contacts/import/monica".
  basePath: string;
  // Source-specific: kick off the background fetch for a connected session.
  startFetch: (sessionId: string) => Promise<void>;
}

export interface SourceImportWizard {
  step: SourceImportStep;
  status: SourceImportStatus | null;
  preview: SourceImportPreviewResponse | null;
  result: SourceImportResult | null;
  rowActions: Map<number, RowSourceAction>;
  error: string | null;
  busy: boolean;
  sessionId: string | null;
  beginFetch: (sessionId: string) => Promise<void>;
  setRowAction: (rowIndex: number, action: RowSourceAction) => void;
  setAllActions: (action: RowSourceAction) => void;
  confirm: () => Promise<void>;
  // cancelImport aborts an in-flight import (importing step); the poll loop
  // then returns to the review step once the backend has rolled back.
  cancelImport: () => Promise<void>;
  cancel: () => Promise<void>;
  reset: () => void;
}

// The effective action for a row: an explicit user choice, else the server's
// suggested_action (which already accounts for validation errors and
// duplicates), else "add". Exported so the review UI shows the same value the
// confirm will send.
export function resolveRowAction(
  rowActions: Map<number, RowSourceAction>,
  row: { row_index: number; suggested_action?: string },
): RowSourceAction {
  const chosen = rowActions.get(row.row_index);
  if (chosen) return chosen;
  if (row.suggested_action === 'skip' || row.suggested_action === 'update') {
    return row.suggested_action;
  }
  return 'add';
}

export function useSourceImportWizard({
  basePath,
  startFetch,
}: UseSourceImportWizardOptions): SourceImportWizard {
  const [step, setStep] = useState<SourceImportStep>('connect');
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [status, setStatus] = useState<SourceImportStatus | null>(null);
  const [preview, setPreview] = useState<SourceImportPreviewResponse | null>(null);
  const [result, setResult] = useState<SourceImportResult | null>(null);
  const [rowActions, setRowActions] = useState<Map<number, RowSourceAction>>(new Map());
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const stopPolling = useCallback(() => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current);
      pollTimer.current = null;
    }
  }, []);

  useEffect(() => stopPolling, [stopPolling]);

  const poll = useCallback(
    async (sid: string, forStep: 'fetching' | 'importing') => {
      let st: SourceImportStatus;
      try {
        st = await getSourceImportStatus(basePath, sid);
      } catch (err) {
        stopPolling();
        setError(getErrorMessage(err));
        return;
      }
      setStatus(st);

      if (st.phase === 'failed') {
        stopPolling();
        setError(st.error || 'The import failed.');
        return;
      }

      if (forStep === 'fetching' && st.phase === 'ready') {
        stopPolling();
        try {
          setPreview(await getSourceImportPreview(basePath, sid));
          setStep('review');
        } catch (err) {
          setError(getErrorMessage(err));
        }
        return;
      }

      if (forStep === 'importing' && st.phase === 'cancelled') {
        // The in-flight import was rolled back; the session is retryable.
        stopPolling();
        setError(null);
        setStep('review');
        return;
      }

      if (forStep === 'importing' && st.phase === 'done') {
        stopPolling();
        if (st.result) setResult(st.result);
        setStep('result');
      }
    },
    [basePath, stopPolling],
  );

  const startPolling = useCallback(
    (sid: string, forStep: 'fetching' | 'importing') => {
      stopPolling();
      void poll(sid, forStep);
      pollTimer.current = setInterval(() => {
        void poll(sid, forStep);
      }, POLL_INTERVAL_MS);
    },
    [poll, stopPolling],
  );

  const beginFetch = useCallback(
    async (sid: string) => {
      setSessionId(sid);
      setError(null);
      setBusy(true);
      try {
        await startFetch(sid);
        setStep('fetching');
        startPolling(sid, 'fetching');
      } catch (err) {
        setError(getErrorMessage(err));
      } finally {
        setBusy(false);
      }
    },
    [startFetch, startPolling],
  );

  const setRowAction = useCallback((rowIndex: number, action: RowSourceAction) => {
    setRowActions((prev) => {
      const next = new Map(prev);
      next.set(rowIndex, action);
      return next;
    });
  }, []);

  const setAllActions = useCallback(
    (action: RowSourceAction) => {
      const next = new Map<number, RowSourceAction>();
      for (const row of preview?.rows ?? []) {
        // "update" only makes sense where a duplicate was detected; other
        // rows fall back to "add" so a bulk "merge all" never sends an
        // unresolvable update.
        if (action === 'update' && !row.duplicate_match) {
          next.set(row.row_index, 'add');
        } else {
          next.set(row.row_index, action);
        }
      }
      setRowActions(next);
    },
    [preview],
  );

  // confirm starts the background import (the endpoint returns 202) and moves
  // to the importing step; the poll loop takes it to result / review / error.
  const confirm = useCallback(async () => {
    if (!sessionId || !preview) return;
    setError(null);
    setBusy(true);
    const actions = preview.rows.map((row) => ({
      row_index: row.row_index,
      action: resolveRowAction(rowActions, row),
    }));
    try {
      await confirmSourceImport(basePath, sessionId, actions);
      setStep('importing');
      startPolling(sessionId, 'importing');
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setBusy(false);
    }
  }, [basePath, preview, rowActions, sessionId, startPolling]);

  // cancelImport aborts an in-flight import: the backend rolls the transaction
  // back and the poll loop returns the wizard to the review step.
  const cancelImport = useCallback(async () => {
    if (!sessionId) return;
    setBusy(true);
    try {
      await cancelSourceImport(basePath, sessionId);
    } finally {
      setBusy(false);
    }
  }, [basePath, sessionId]);

  const cancel = useCallback(async () => {
    stopPolling();
    if (sessionId) await cancelSourceImport(basePath, sessionId);
  }, [basePath, sessionId, stopPolling]);

  const reset = useCallback(() => {
    stopPolling();
    setStep('connect');
    setSessionId(null);
    setStatus(null);
    setPreview(null);
    setResult(null);
    setRowActions(new Map());
    setError(null);
    setBusy(false);
  }, [stopPolling]);

  return {
    step,
    status,
    preview,
    result,
    rowActions,
    error,
    busy,
    sessionId,
    beginFetch,
    setRowAction,
    setAllActions,
    confirm,
    cancelImport,
    cancel,
    reset,
  };
}
