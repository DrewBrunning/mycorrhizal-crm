import { type RefObject, useCallback, useEffect, useLayoutEffect, useRef } from 'react';

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
  // `target` `null` means go straight to the fallback. `since` is the list the
  // user saw when they acted: only a change away from it is the removal.
  const pending = useRef<{ target: Key | null; since: unknown } | undefined>(undefined);
  // The list as last committed to the DOM. A layout effect runs during the
  // commit, so this is current by the time any event handler can run, whereas
  // the passive effect below can still be queued for that same commit (#2920).
  const committed = useRef<unknown>(dependency);
  useLayoutEffect(() => {
    committed.current = dependency;
  }, [dependency]);

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
    pending.current = {
      target: neighbour === undefined ? null : neighbour,
      since: committed.current,
    };
  }, []);

  const cancel = useCallback(() => {
    pending.current = undefined;
  }, []);

  useEffect(() => {
    const request = pending.current;
    // A passive effect still queued from a commit before the request is not
    // the removal: without this check a fast click on a freshly loaded list
    // moved focus even when the action then failed.
    if (request === undefined || !ready || dependency === request.since) return;
    pending.current = undefined;
    const { target } = request;
    const el = target === null ? null : elements.current.get(target);
    (el ?? fallback.current)?.focus();
  }, [dependency, fallback, ready]);

  return { register, request, cancel };
}
