import type { AppAction, AppInfo, AppPrefsPatch, AppsResponse } from './types';

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let message = res.statusText;
    try {
      message = ((await res.json()) as { error?: string }).error ?? message;
    } catch {
      // not JSON
    }
    throw new ApiError(res.status, message);
  }
  return (res.status === 204 ? undefined : await res.json()) as T;
}

export const api = {
  apps: () => request<AppsResponse>('GET', '/api/apps'),
  action: (id: string, action: AppAction) =>
    request<AppInfo>('POST', `/api/apps/${id}/${action}`),
  patch: (id: string, prefs: AppPrefsPatch) =>
    request<AppInfo>('PATCH', `/api/apps/${id}`, prefs),
  /** Shell UI state; null when never saved. */
  getState: async <T>(key: string): Promise<T | null> => {
    try {
      return await request<T>('GET', `/api/state/${key}`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return null;
      throw e;
    }
  },
  putState: (key: string, value: unknown) =>
    request<void>('PUT', `/api/state/${key}`, value),
};
