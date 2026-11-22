// Pages stay mounted once visited and are hidden with CSS (see Pages in
// app.tsx). A sheet renders outside its page (a portal), so CSS can't hide
// it: Sheet here is only open while its page is the one on screen.
import { Sheet as UISheet } from '@hub/ui-kit';
import { createContext, useContext, type ComponentProps } from 'react';

/** Whether the page this component belongs to is the visible one. */
export const PageActive = createContext(true);

export function Sheet({ open, ...props }: ComponentProps<typeof UISheet>) {
  const active = useContext(PageActive);
  return <UISheet open={!!open && active} {...props} />;
}
