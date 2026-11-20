import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, type RenderResult } from '@testing-library/react';
import type { ReactNode } from 'react';
import { appsKey } from '../api/apps';
import type { AppInfo, AppsResponse } from '../api/types';

export function app(id: string, over: Partial<AppInfo> = {}): AppInfo {
  return {
    id,
    name: id[0].toUpperCase() + id.slice(1),
    description: '',
    url: `http://${id}.hub.localhost:7000`,
    uiPath: '/',
    keepAlive: true,
    autoStart: false,
    pinned: false,
    position: null,
    status: 'stopped',
    since: '2026-11-20T00:00:00Z',
    ...over,
  };
}

export function renderWithApps(
  ui: ReactNode,
  apps: AppInfo[],
): RenderResult & { qc: QueryClient } {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData<AppsResponse>(appsKey, { apps, problems: [] });
  return {
    qc,
    ...render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>),
  };
}
