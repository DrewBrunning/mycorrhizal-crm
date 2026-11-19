import { useEffect, useRef, useState } from 'react';
import { getUserDirectory, type UserDirectoryEntry } from '../api/contactShares';
import { getErrorMessage } from '../utils/errorHandler';
import { useDebouncedValue } from './useDebounce';

// Backend SHARE_DIRECTORY_MODE=search needs at least this many characters of the username.
export const SHARE_SEARCH_MIN_CHARS = 3;
const SEARCH_DEBOUNCE_MS = 250;

// Drives the share recipient pickers against GET /users/directory?q=.
//
// The response does not say which SHARE_DIRECTORY_MODE the operator chose, so it is inferred:
// a query shorter than 3 characters (the initial, empty one included) that returns no users
// means `search` mode. In `all` mode (the default) the server ignores q and lists everyone, so
// the picker behaves exactly as before. Once search mode is known, queries shorter than 3
// characters are not sent. Only the newest response is applied.
export function useShareRecipientSearch(onError: (message: string) => void) {
  const [query, setQuery] = useState('');
  const [users, setUsers] = useState<UserDirectoryEntry[]>([]);
  const [searchMode, setSearchMode] = useState(false);
  const searchModeRef = useRef(false);
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;
  const debounced = useDebouncedValue(query.trim(), SEARCH_DEBOUNCE_MS);
  const tooShort = debounced.length < SHARE_SEARCH_MIN_CHARS;

  useEffect(() => {
    if (searchModeRef.current && tooShort) {
      setUsers([]);
      return;
    }
    let cancelled = false;
    getUserDirectory(debounced)
      .then((list) => {
        if (cancelled) return;
        setUsers(list);
        if (tooShort) {
          searchModeRef.current = list.length === 0;
          setSearchMode(list.length === 0);
        }
      })
      .catch((err) => {
        if (!cancelled) onErrorRef.current(getErrorMessage(err));
      });
    return () => {
      cancelled = true;
    };
  }, [debounced, tooShort]);

  return { users, searchMode, query, setQuery };
}
