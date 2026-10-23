import Box from '@mui/material/Box';
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';

const AnnouncerContext = createContext<{ announce: (msg: string) => void } | undefined>(undefined);

export function AnnouncerProvider({ children }: { children: ReactNode }) {
  const [message, setMessage] = useState('');
  // Tracks the pending 50ms timer so it can be cancelled on unmount (a real
  // component unmount, or -- in tests -- between test cases) and so a second
  // announce() arriving inside the window replaces rather than stacks.
  const timeoutRef = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (timeoutRef.current !== null) window.clearTimeout(timeoutRef.current);
    },
    [],
  );

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
        // Role status carries an implicit polite live region. Orca announces
        // changes to a status role reliably; the same text in a bare
        // aria-live box was exposed in the AX tree but never spoken (#1918).
        role="status"
        aria-live="polite"
        aria-atomic="true"
        // Keep the region mounted and visually hidden but *showing* to AT:
        // display:none/visibility:hidden would remove it from the AX tree.
        sx={{
          position: 'absolute',
          // Pixel strings, not `1`: MUI's sx reads a bare number in 0..1 as a
          // *fraction* (width: 1 = 100%). This live region was therefore as
          // wide and tall as its containing block, and being absolutely
          // positioned below the app it stretched every page's scroll height by
          // a viewport of blank space (#1788).
          width: '1px',
          height: '1px',
          p: 0,
          m: '-1px',
          border: 0,
          overflow: 'hidden',
          clip: 'rect(0 0 0 0)',
          clipPath: 'inset(50%)',
          whiteSpace: 'nowrap',
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
