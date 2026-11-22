import { applyThemeClass } from '@hub/app-sdk';
import { useEffect } from 'react';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type Theme = 'system' | 'light' | 'dark';

interface UIState {
  theme: Theme;
  setTheme: (t: Theme) => void;
  /** App whose logs drawer is open. */
  logsFor: string | null;
  showLogs: (id: string | null) => void;
}

export const useUI = create<UIState>()(
  persist(
    (set) => ({
      theme: 'system',
      setTheme: (theme) => set({ theme }),
      logsFor: null,
      showLogs: (logsFor) => set({ logsFor }),
    }),
    // index.html reads this key before first paint.
    { name: 'hub-ui', partialize: (s) => ({ theme: s.theme }) },
  ),
);

const prefersDark = () => window.matchMedia('(prefers-color-scheme: dark)');

export function resolvedTheme(t: Theme): 'light' | 'dark' {
  return t === 'system' ? (prefersDark().matches ? 'dark' : 'light') : t;
}

/** Keeps <html class="dark"> in sync with the chosen theme and the OS. */
export function useApplyTheme() {
  const theme = useUI((s) => s.theme);
  useEffect(() => {
    const apply = () => applyThemeClass(resolvedTheme(theme));
    apply();
    const mq = prefersDark();
    mq.addEventListener('change', apply);
    return () => mq.removeEventListener('change', apply);
  }, [theme]);
}
