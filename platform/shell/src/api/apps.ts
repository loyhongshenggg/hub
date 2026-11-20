import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { toast } from '../state/toasts';
import { api } from './client';
import type { AppAction, AppInfo, AppPrefsPatch, AppsResponse } from './types';

export const appsKey = ['apps'] as const;

export function useApps() {
  return useQuery({
    queryKey: appsKey,
    queryFn: api.apps,
    staleTime: Infinity,
  });
}

export function useApp(id: string | null): AppInfo | undefined {
  const { data } = useApps();
  return id ? data?.apps.find((a) => a.id === id) : undefined;
}

/**
 * Keeps the apps query live from the core's SSE stream (/api/events), which
 * sends the full list on connect and after every change. Returns whether the
 * stream is connected; EventSource reconnects on its own.
 */
export function useAppsStream(): boolean {
  const qc = useQueryClient();
  const [connected, setConnected] = useState(true);
  useEffect(() => {
    const es = new EventSource('/api/events');
    es.addEventListener('apps', (e) => {
      qc.setQueryData(
        appsKey,
        JSON.parse((e as MessageEvent<string>).data) as AppsResponse,
      );
      setConnected(true);
    });
    es.onerror = () => setConnected(false);
    return () => es.close();
  }, [qc]);
  return connected;
}

function updateApp(qc: QueryClient, info: AppInfo) {
  qc.setQueryData<AppsResponse>(
    appsKey,
    (old) =>
      old && {
        ...old,
        apps: old.apps.map((a) => (a.id === info.id ? info : a)),
      },
  );
}

export function useAppAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: AppAction }) =>
      api.action(id, action),
    onSuccess: (info) => updateApp(qc, info),
    onError: (e) => toast(e.message),
  });
}

export function usePatchApp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, prefs }: { id: string; prefs: AppPrefsPatch }) =>
      api.patch(id, prefs),
    onSuccess: (info) => updateApp(qc, info),
    onError: (e) => toast(e.message),
  });
}

export const isOn = (a: AppInfo) =>
  a.status === 'running' || a.status === 'starting';

const byName = (a: AppInfo, b: AppInfo) => a.name.localeCompare(b.name);

/** Dock order: pinned apps (by position, then name), then other running apps. */
export function dockApps(apps: AppInfo[]): {
  pinned: AppInfo[];
  running: AppInfo[];
} {
  const pinned = apps
    .filter((a) => a.pinned)
    .sort(
      (a, b) =>
        (a.position ?? Infinity) - (b.position ?? Infinity) || byName(a, b),
    );
  const running = apps
    .filter((a) => !a.pinned && a.status !== 'stopped')
    .sort(byName);
  return { pinned, running };
}

export const sortByName = (apps: AppInfo[]) => [...apps].sort(byName);
