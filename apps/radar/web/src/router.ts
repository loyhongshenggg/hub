// A tiny hash router. Navigation happens inside a React transition so the
// <ViewTransition> boundaries animate between screens.
import { addTransitionType, startTransition, useEffect, useState } from 'react';

export type Route =
  | { name: 'home' }
  | { name: 'track'; track: string }
  | { name: 'lesson'; track: string; chapter: string; step?: string }
  | { name: 'news' }
  | { name: 'sources' }
  | { name: 'history' }
  | { name: 'settings' }
  | { name: 'assess' };

export function parse(hash: string): Route {
  const [path, qs] = hash.replace(/^#\/?/, '').split('?');
  const parts = path.split('/').filter(Boolean).map(decodeURIComponent);
  const q = new URLSearchParams(qs ?? '');
  switch (parts[0]) {
    case 'track':
      if (parts[1]) return { name: 'track', track: parts[1] };
      break;
    case 'learn':
      if (parts[1] && parts[2])
        return {
          name: 'lesson',
          track: parts[1],
          chapter: parts[2],
          step: q.get('step') ?? undefined,
        };
      break;
    case 'news':
    case 'sources':
    case 'history':
    case 'settings':
    case 'assess':
      return { name: parts[0] };
  }
  return { name: 'home' };
}

export function href(r: Route): string {
  const e = encodeURIComponent;
  switch (r.name) {
    case 'home':
      return '#/';
    case 'track':
      return `#/track/${e(r.track)}`;
    case 'lesson':
      return `#/learn/${e(r.track)}/${e(r.chapter)}${r.step ? `?step=${e(r.step)}` : ''}`;
    default:
      return `#/${r.name}`;
  }
}

const depth: Record<Route['name'], number> = {
  home: 0,
  news: 1,
  sources: 2,
  history: 1,
  settings: 1,
  assess: 1,
  track: 1,
  lesson: 2,
};

/** navigate pushes (or replaces) a route; the hashchange listener renders it. */
export function navigate(r: Route, opts: { replace?: boolean } = {}) {
  const h = href(r);
  if (opts.replace) window.location.replace(h);
  else window.location.hash = h;
}

export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(() => parse(window.location.hash));
  useEffect(() => {
    let current = route;
    const onChange = () => {
      const next = parse(window.location.hash);
      const same = next.name === current.name;
      startTransition(() => {
        // Step changes inside a lesson slide; screen changes move by depth.
        addTransitionType(
          same && next.name === 'lesson'
            ? 'step'
            : depth[next.name] >= depth[current.name]
              ? 'forward'
              : 'back',
        );
        setRoute(next);
      });
      current = next;
    };
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return route;
}
