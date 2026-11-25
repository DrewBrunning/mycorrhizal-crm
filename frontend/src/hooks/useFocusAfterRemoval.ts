import { type RefObject, useCallback, useEffect, useRef } from 'react';

type Key = string | number;

/**
 * Restores focus after an action unmounts the row that held it (#2918, #2920).
 *
 * Register each row's action button with `register(key)`, call `request` with
 * the row order and the row about to disappear, and pass the list that changes
 * as `dependency`. Once it changes, focus moves to the next row's same action
 * button, else the previous row's, else `fallback` (a heading or empty state
 * with `tabIndex={-1}`). If the action fails, call `cancel` so a later,
 * unrelated list change does not steal focus.
 */
export function useFocusAfterRemoval(
  fallback: RefObject<HTMLElement | null>,
  dependency: unknown,
  // False while the list is mid-reload (e.g. cleared and refetched): the
  // pending request waits until the rows are back.
  ready = true,
) {
  const elements = useRef(new Map<Key, HTMLElement>());
  // `undefined`: no request. `null`: go straight to the fallback.
  const pending = useRef<Key | null | undefined>(undefined);

  const register = useCallback(
    (key: Key) => (el: HTMLElement | null) => {
      if (el) elements.current.set(key, el);
      else elements.current.delete(key);
    },
    [],
  );

  const request = useCallback((order: readonly Key[], removed: Key) => {
    const index = order.indexOf(removed);
    const neighbour = index < 0 ? undefined : (order[index + 1] ?? order[index - 1]);
    pending.current = neighbour === undefined ? null : neighbour;
  }, []);

  const cancel = useCallback(() => {
    pending.current = undefined;
  }, []);

  useEffect(() => {
    const target = pending.current;
    if (target === undefined || !ready) return;
    pending.current = undefined;
    const el = target === null ? null : elements.current.get(target);
    (el ?? fallback.current)?.focus();
  }, [dependency, fallback, ready]);

  return { register, request, cancel };
}
