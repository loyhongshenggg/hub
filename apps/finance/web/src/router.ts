// A tiny hash router: #/<page>[/<id>][?query].
import { useEffect, useState } from 'react';

export type Page = 'overview' | 'goals' | 'transactions' | 'import' | 'settings';

export interface Route {
  page: Page;
  id?: string;
  q: URLSearchParams;
}

export const pages: Page[] = ['overview', 'goals', 'transactions', 'import', 'settings'];

export function parse(hash: string): Route {
  const [path, query] = hash.replace(/^#\/?/, '').split('?');
  const parts = path.split('/').filter(Boolean).map(decodeURIComponent);
  const page = (pages as string[]).includes(parts[0] ?? '') ? (parts[0] as Page) : 'overview';
  return { page, id: parts[1], q: new URLSearchParams(query ?? '') };
}

export function href(page: Page, id?: string | number, q?: Record<string, string | number | boolean | undefined>): string {
  let h = `#/${page}`;
  if (id !== undefined && id !== '') h += `/${encodeURIComponent(String(id))}`;
  if (q) {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(q)) {
      if (v === undefined || v === '' || v === false) continue;
      p.set(k, v === true ? '1' : String(v));
    }
    const s = p.toString();
    if (s) h += `?${s}`;
  }
  return h;
}

export function go(page: Page, id?: string | number, q?: Record<string, string | number | boolean | undefined>, replace = false) {
  const h = href(page, id, q);
  if (replace) window.location.replace(h);
  else window.location.hash = h;
}

/**
 * The current route, updated urgently on every hash change so the tab bar
 * reacts at once. Render page content from useDeferredValue(route) (app.tsx).
 */
export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(() => parse(window.location.hash));
  useEffect(() => {
    const on = () => setRoute(parse(window.location.hash));
    window.addEventListener('hashchange', on);
    return () => window.removeEventListener('hashchange', on);
  }, []);
  return route;
}
