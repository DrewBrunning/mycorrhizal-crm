import { createContext, useContext } from 'react';

/**
 * The id of the enclosing contact-detail panel's visible title. A control whose
 * name repeats across panels ("Add" on the agenda, gifts and clothing sizes)
 * points `aria-describedby` at it, so assistive tech can tell the controls
 * apart (#2920 repeated-name check). Undefined outside a panel.
 */
export const PanelTitleIdContext = createContext<string | undefined>(undefined);

export function usePanelTitleId(): string | undefined {
  return useContext(PanelTitleIdContext);
}
