import { Alert, Box, Button } from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router';
import { createUnassignedNote } from './api/notes';
import AddNoteDialog from './components/AddNoteDialog';
import { handleError } from './utils/errorHandler';
import { takeSharedDraft } from './utils/shareTarget';

type DraftState = { status: 'loading' } | { status: 'missing' } | { status: 'ready'; text: string };

// Issue #1294: landing page of the Web Share Target. The service worker parked
// the sanitised shared text in the Cache API; it is taken exactly once here and
// only prefills a note the user picks a contact for and saves by hand. Backing
// out discards it.
export default function ShareTargetPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const key = params.get('key');
  const [draft, setDraft] = useState<DraftState>({ status: 'loading' });
  const saved = useRef(false);
  // The take is destructive, so a re-run of the effect (StrictMode) must reuse
  // the first read rather than finding the entry already gone.
  const pending = useRef<Promise<string | null> | null>(null);

  useEffect(() => {
    let cancelled = false;
    pending.current ??= takeSharedDraft(key).catch(() => null);
    void pending.current.then((text) => {
      if (cancelled) return;
      setDraft(text === null ? { status: 'missing' } : { status: 'ready', text });
    });
    return () => {
      cancelled = true;
    };
  }, [key]);

  const handleSave = async (content: string, date: string, contactId?: number) => {
    try {
      await createUnassignedNote({
        content,
        date: new Date(date).toISOString(),
        contact_id: contactId,
      });
      saved.current = true;
    } catch (err) {
      handleError(err, { operation: 'creating note' });
      throw err;
    }
  };

  if (draft.status === 'loading') return null;

  if (draft.status === 'missing') {
    return (
      <Box
        sx={{ p: 2, display: 'flex', flexDirection: 'column', gap: 2, alignItems: 'flex-start' }}
      >
        <Alert severity="info">{t('shareTarget.missing')}</Alert>
        <Button variant="outlined" onClick={() => void navigate('/', { replace: true })}>
          {t('shareTarget.backHome')}
        </Button>
      </Box>
    );
  }

  return (
    <AddNoteDialog
      open
      initialContent={draft.text}
      onSave={handleSave}
      onClose={() => void navigate(saved.current ? '/notes' : '/', { replace: true })}
    />
  );
}
