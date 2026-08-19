import { createContext, useContext, useState, useCallback, useEffect, useRef, ReactNode } from 'react';
import Box from '@mui/material/Box';

const AnnouncerContext = createContext<{ announce: (msg: string) => void } | undefined>(undefined);

export function AnnouncerProvider({ children }: { children: ReactNode }) {
  const [message, setMessage] = useState('');
  // Tracks the pending 50ms timer so it can be cancelled on unmount (a real
  // component unmount, or -- in tests -- between test cases) and so a second
  // announce() arriving inside the window replaces rather than stacks.
  const timeoutRef = useRef<number | null>(null);

  useEffect(() => () => {
    if (timeoutRef.current !== null) window.clearTimeout(timeoutRef.current);
  }, []);

  const announce = useCallback((msg: string) => {
    if (timeoutRef.current !== null) window.clearTimeout(timeoutRef.current);
    // Clear first so repeating the same string still fires a change.
    setMessage('');
    timeoutRef.current = window.setTimeout(() => setMessage(msg), 50);
  }, []);

  return (
    <AnnouncerContext.Provider value={{ announce }}>
      {children}
      <Box
        aria-live="polite"
        aria-atomic="true"
        sx={{
          position: 'absolute', width: 1, height: 1, overflow: 'hidden',
          clip: 'rect(0 0 0 0)', clipPath: 'inset(50%)', whiteSpace: 'nowrap',
        }}
      >
        {message}
      </Box>
    </AnnouncerContext.Provider>
  );
}

export function useAnnouncer() {
  const ctx = useContext(AnnouncerContext);
  if (!ctx) throw new Error('useAnnouncer must be used within an AnnouncerProvider');
  return ctx;
}
