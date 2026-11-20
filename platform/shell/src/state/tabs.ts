import { create } from 'zustand';
import { api } from '../api/client';

/** Open app tabs. `active` null = the Home tab. Saved in the core (/api/state/tabs). */
export interface TabsState {
  open: string[];
  active: string | null;
  /** Start path for a tab opened via openApp(id, path); used once, when its iframe mounts. */
  paths: Record<string, string>;
  openApp: (id: string, path?: string) => void;
  /** Read and forget the start path of a tab. */
  takePath: (id: string) => string | undefined;
  closeApp: (id: string) => void;
  activate: (id: string | null) => void;
  /** Drop tabs of apps that no longer exist. */
  prune: (known: Set<string>) => void;
}

export const useTabs = create<TabsState>((set, get) => ({
  open: [],
  active: null,
  paths: {},
  openApp: (id, path) =>
    set((s) =>
      s.open.includes(id)
        ? { active: id }
        : {
            open: [...s.open, id],
            active: id,
            paths: path ? { ...s.paths, [id]: path } : s.paths,
          },
    ),
  takePath: (id) => {
    const path = get().paths[id];
    if (path !== undefined) {
      const paths = { ...get().paths };
      delete paths[id];
      set({ paths });
    }
    return path;
  },
  closeApp: (id) =>
    set((s) => {
      const i = s.open.indexOf(id);
      if (i < 0) return s;
      const open = s.open.filter((x) => x !== id);
      const active =
        s.active === id ? (open[i] ?? open[i - 1] ?? null) : s.active;
      return { open, active };
    }),
  activate: (id) => set({ active: id }),
  prune: (known) =>
    set((s) => {
      const open = s.open.filter((id) => known.has(id));
      if (open.length === s.open.length) return s;
      return {
        open,
        active: s.active && known.has(s.active) ? s.active : null,
      };
    }),
}));

interface Saved {
  open?: unknown;
  active?: unknown;
}

/** Restore tabs from the core, then save every change (debounced). */
export async function persistTabs(): Promise<() => void> {
  try {
    const saved = await api.getState<Saved>('tabs');
    if (saved && Array.isArray(saved.open)) {
      const open = saved.open.filter((x): x is string => typeof x === 'string');
      const active =
        typeof saved.active === 'string' && open.includes(saved.active)
          ? saved.active
          : null;
      useTabs.setState({ open, active });
    }
  } catch {
    // Core unreachable: start empty; saving below retries on the next change.
  }
  let timer: ReturnType<typeof setTimeout> | undefined;
  return useTabs.subscribe((s, prev) => {
    if (s.open === prev.open && s.active === prev.active) return;
    clearTimeout(timer);
    timer = setTimeout(
      () =>
        void api
          .putState('tabs', { open: s.open, active: s.active })
          .catch(() => undefined),
      300,
    );
  });
}
