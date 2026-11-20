import { create } from 'zustand';

/** What apps told the shell over the bridge (badges, tab titles). */
interface BridgeState {
  badges: Record<string, number>;
  titles: Record<string, string>;
  setBadge: (appId: string, count: number) => void;
  setTitle: (appId: string, title: string | null) => void;
  /** Forget everything an app said (e.g. when it stops). */
  reset: (appId: string) => void;
}

const without = <T>(rec: Record<string, T>, key: string) => {
  if (!(key in rec)) return rec;
  const next = { ...rec };
  delete next[key];
  return next;
};

export const useBridgeState = create<BridgeState>((set) => ({
  badges: {},
  titles: {},
  setBadge: (appId, count) =>
    set((s) => ({
      badges:
        count > 0 ? { ...s.badges, [appId]: count } : without(s.badges, appId),
    })),
  setTitle: (appId, title) =>
    set((s) => ({
      titles: title
        ? { ...s.titles, [appId]: title }
        : without(s.titles, appId),
    })),
  reset: (appId) =>
    set((s) => ({
      badges: without(s.badges, appId),
      titles: without(s.titles, appId),
    })),
}));
