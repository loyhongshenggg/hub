import type { HubClient } from '@hub/app-sdk';
import { cn, TooltipProvider } from '@hub/ui-kit';
import { QueryClientProvider } from '@tanstack/react-query';
import { FileUp, LayoutDashboard, ListOrdered, Settings as SettingsIcon, Target } from 'lucide-react';
import { Activity, memo, useDeferredValue, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { api } from './api';
import { GoalsPage } from './goals';
import { ImportPage } from './import';
import { OverviewPage } from './overview';
import { keys, queryClient, useStatus } from './queries';
import { href, pages, parse, useRoute, type Page, type Route } from './router';
import { SettingsPage } from './settings';
import { TransactionsPage, txnQueryFor } from './transactions';

export function App({ hub }: { hub: HubClient }) {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <Shell hub={hub} />
      </TooltipProvider>
    </QueryClientProvider>
  );
}

const tabs: { page: Page; name: string; icon: typeof Target }[] = [
  { page: 'overview', name: 'Overview', icon: LayoutDashboard },
  { page: 'goals', name: 'Goals', icon: Target },
  { page: 'transactions', name: 'Transactions', icon: ListOrdered },
  { page: 'import', name: 'Import', icon: FileUp },
];

function Shell({ hub }: { hub: HubClient }) {
  // The tab bar follows `route` at once; page content follows `shown`, which
  // React renders in the background so a click never waits on a big page.
  const route = useRoute();
  const shown = useDeferredValue(route);
  const { data: status } = useStatus();
  const review = status?.review ?? 0;

  useEffect(() => {
    hub.setBadge(review);
  }, [hub, review]);

  return (
    <div className="cq min-h-dvh">
      <header className="sticky top-0 z-30 border-b bg-background/85 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center gap-2 px-3 sm:px-5">
          <a href={href('overview')} className="mr-2 flex items-center gap-2 py-3 font-semibold tracking-tight">
            <img src="/icon.svg" alt="" className="size-6" />
            <span className="max-[560px]:sr-only">Finance</span>
          </a>
          <nav className="-mb-px flex flex-1 overflow-x-auto" aria-label="Sections">
            {tabs.map((t) => {
              const active = route.page === t.page;
              const Icon = t.icon;
              return (
                <a
                  key={t.page}
                  href={href(t.page)}
                  aria-current={active ? 'page' : undefined}
                  className={cn(
                    'flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-3 text-sm transition-colors duration-[var(--dur-fast)]',
                    active ? 'border-primary font-medium text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
                  )}
                >
                  <Icon className="size-4" aria-hidden />
                  <span className={active ? 'max-[400px]:sr-only' : 'max-[640px]:sr-only'}>{t.name}</span>
                  {t.page === 'transactions' && review > 0 && (
                    <span className="tabular rounded-full bg-[var(--warn)]/25 px-1.5 text-[11px] font-medium text-foreground" title={`${review} to check`}>
                      {review}
                    </span>
                  )}
                </a>
              );
            })}
          </nav>
          <a
            href={href('settings')}
            aria-label="Settings"
            aria-current={route.page === 'settings' ? 'page' : undefined}
            className={cn('rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-foreground', route.page === 'settings' && 'bg-accent text-foreground')}
          >
            <SettingsIcon className="size-4" />
          </a>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-3 py-5 sm:px-5 sm:py-6">
        <Pages shown={shown} />
      </main>
    </div>
  );
}

/**
 * Every page stays mounted once rendered, hidden in an <Activity>, so going
 * back to a tab only unhides it (state and scroll position kept). Once the
 * first page is up and the browser is idle, the other pages' data is
 * prefetched and they are pre-rendered hidden, at low priority, so even the
 * first visit to a tab is instant.
 */
function Pages({ shown }: { shown: Route }) {
  const [routes, setRoutes] = useState<Partial<Record<Page, Route>>>(() => ({ [shown.page]: shown }));
  if (routes[shown.page] !== shown) setRoutes({ ...routes, [shown.page]: shown });
  const [warm, setWarm] = useState(false);
  useEffect(
    () =>
      whenIdle(() => {
        // Hidden pages don't subscribe to queries, so render them once the
        // data is in the cache.
        void prefetchPages().then(() => setWarm(true));
      }),
    [],
  );

  // Each page keeps its own scroll position.
  const scroll = useRef<Partial<Record<Page, number>>>({});
  const prev = useRef(shown.page);
  useLayoutEffect(() => {
    if (prev.current === shown.page) return;
    window.scrollTo(0, scroll.current[shown.page] ?? 0);
    prev.current = shown.page;
  }, [shown.page]);
  useEffect(() => {
    const on = () => (scroll.current[prev.current] = window.scrollY);
    window.addEventListener('scroll', on, { passive: true });
    return () => window.removeEventListener('scroll', on);
  }, []);

  return pages.map((p) => {
    const r = routes[p] ?? (warm ? home[p] : undefined);
    return r ? (
      <Activity key={p} mode={p === shown.page ? 'visible' : 'hidden'}>
        <div data-page={p}>
          <PageView route={r} />
        </div>
      </Activity>
    ) : null;
  });
}

const home = Object.fromEntries(pages.map((p) => [p, parse(`#/${p}`)])) as Record<Page, Route>;

const PageView = memo(function PageView({ route }: { route: Route }) {
  switch (route.page) {
    case 'overview':
      return <OverviewPage route={route} />;
    case 'goals':
      return <GoalsPage route={route} />;
    case 'transactions':
      return <TransactionsPage route={route} />;
    case 'import':
      return <ImportPage route={route} />;
    case 'settings':
      return <SettingsPage />;
  }
});

/** What each page shows first, fetched ahead of a visit (see Pages). */
function prefetchPages(): Promise<unknown> {
  const q = queryClient;
  const txns = txnQueryFor(home.transactions.q);
  return Promise.all([
    q.prefetchQuery({ queryKey: keys.overview(undefined, false), queryFn: () => api.overview(undefined, false) }),
    q.prefetchQuery({ queryKey: keys.goals, queryFn: api.goals }),
    q.prefetchQuery({ queryKey: keys.txns(txns), queryFn: () => api.txns(txns) }),
    q.prefetchQuery({ queryKey: keys.statements, queryFn: api.statements }),
    q.prefetchQuery({ queryKey: keys.claude, queryFn: api.claude }),
    q.prefetchQuery({ queryKey: keys.rules, queryFn: api.rules }),
    q.prefetchQuery({ queryKey: ['people'], queryFn: api.people }),
  ]);
}

function whenIdle(fn: () => void): () => void {
  if ('requestIdleCallback' in window) {
    const id = requestIdleCallback(fn, { timeout: 2000 });
    return () => cancelIdleCallback(id);
  }
  const id = setTimeout(fn, 500);
  return () => clearTimeout(id);
}
