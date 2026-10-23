import { useEffect, useState } from 'react';

/**
 * Tracks `navigator.onLine` and re-renders on the browser's online/offline
 * events (issue #1915). The initial value is read lazily so an offline reload
 * shows the offline state on its very first paint.
 */
export function useOnlineStatus(): boolean {
  const [online, setOnline] = useState(() =>
    typeof navigator === 'undefined' ? true : navigator.onLine,
  );

  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    // The status can have changed between the initial render and subscribing.
    update();
    window.addEventListener('online', update);
    window.addEventListener('offline', update);
    return () => {
      window.removeEventListener('online', update);
      window.removeEventListener('offline', update);
    };
  }, []);

  return online;
}
