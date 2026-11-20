// Typed client for the Radar Go API (apps/radar/api).

export type CategoryId = 'swe' | 'security' | 'ai';

export interface Item {
  id: number;
  title: string;
  url: string;
  summary: string;
  publishedAt: string;
  read: boolean;
  saved: boolean;
  source: { id: string; name: string; category: CategoryId };
}

export interface Counts {
  unread: number;
  unreadByCategory: Partial<Record<CategoryId, number>>;
  saved: number;
  newSinceVisit: number;
}

export interface FetchStatus {
  running: boolean;
  lastRound: string | null;
}

export interface Source {
  id: string;
  name: string;
  url: string;
  category: CategoryId;
  active: boolean;
  lastFetchAt: string | null;
  lastOkAt: string | null;
  lastError?: string;
  items: number;
}

export interface NewsFilter {
  category: CategoryId | 'all';
  q: string;
  unread: boolean;
  saved: boolean;
}

async function call<T>(
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
    let msg = `${res.status} ${res.statusText}`;
    try {
      msg = ((await res.json()) as { error?: string }).error ?? msg;
    } catch {
      // not JSON
    }
    throw new Error(msg);
  }
  return (
    res.status === 204 || res.status === 202 ? undefined : await res.json()
  ) as T;
}

function query(f: NewsFilter, cursor?: string): string {
  const p = new URLSearchParams({ category: f.category, limit: '40' });
  if (f.q.trim()) p.set('q', f.q.trim());
  if (f.unread) p.set('unread', '1');
  if (f.saved) p.set('saved', '1');
  if (cursor) p.set('cursor', cursor);
  return p.toString();
}

export const api = {
  news: (f: NewsFilter, cursor?: string) =>
    call<{ items: Item[]; nextCursor: string }>(
      'GET',
      `/api/news?${query(f, cursor)}`,
    ),
  setRead: (id: number, read: boolean) =>
    call<void>('POST', `/api/items/${id}/read`, { read }),
  setSaved: (id: number, saved: boolean) =>
    call<void>('POST', `/api/items/${id}/saved`, { saved }),
  readAll: (f: NewsFilter) =>
    call<{ marked: number }>('POST', `/api/news/read-all?${query(f)}`),
  summary: () =>
    call<{ counts: Counts; fetch: FetchStatus }>('GET', '/api/summary'),
  sources: () =>
    call<{ sources: Source[]; fetch: FetchStatus }>('GET', '/api/sources'),
  refresh: () => call<void>('POST', '/api/refresh'),
  visit: () => call<void>('POST', '/api/visit'),
};
