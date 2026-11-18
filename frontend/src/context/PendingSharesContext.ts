import { createContext, useContext } from 'react';

// #2691: the nav badge's pending-share count is owned by the app shell, which
// used to re-read it only when the route changed. Answering a share on
// /shared-with-me does not navigate, so the page calls refreshPendingShares()
// after every mutation that can change the count (the web twin of the shared
// mutation signal Android uses, #2557). The default is a no-op so a page
// rendered outside the shell (tests, isolated mounts) needs no provider.
export interface PendingSharesState {
  refreshPendingShares: () => void;
}

export const PendingSharesContext = createContext<PendingSharesState>({
  refreshPendingShares: () => {},
});

export function usePendingShares(): PendingSharesState {
  return useContext(PendingSharesContext);
}
