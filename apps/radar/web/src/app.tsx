import type { HubClient } from '@hub/app-sdk';
import { cn } from '@hub/ui-kit';
import { Newspaper, Rss } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { api, type Counts, type FetchStatus } from './api';
import { News } from './news';
import { Sources } from './sources';

type View = 'news' | 'sources';

const views: { id: View; name: string; icon: typeof Newspaper }[] = [
  { id: 'news', name: 'News', icon: Newspaper },
  { id: 'sources', name: 'Sources', icon: Rss },
];

export function App({ hub }: { hub: HubClient }) {
  // The URL hash picks the view (#sources), so views can be linked to.
  const [view, setViewState] = useState<View>(() =>
    window.location.hash === '#sources' ? 'sources' : 'news',
  );
  const setView = (v: View) => {
    setViewState(v);
    window.history.replaceState(
      null,
      '',
      v === 'news' ? window.location.pathname : `#${v}`,
    );
  };
  const [counts, setCounts] = useState<Counts | null>(null);
  const [fetchStatus, setFetchStatus] = useState<FetchStatus | null>(null);

  const refreshSummary = useCallback(() => {
    api.summary().then(
      (s) => {
        setCounts(s.counts);
        setFetchStatus((prev) =>
          prev?.lastRound === s.fetch.lastRound &&
          prev.running === s.fetch.running
            ? prev
            : s.fetch,
        );
      },
      () => undefined,
    );
  }, []);

  // Poll faster while a fetch round runs, so new items show up promptly.
  useEffect(() => {
    refreshSummary();
    const t = setInterval(refreshSummary, fetchStatus?.running ? 3000 : 60000);
    return () => clearInterval(t);
  }, [refreshSummary, fetchStatus?.running]);

  return (
    <div className="mx-auto flex min-h-dvh max-w-4xl flex-col gap-5 px-6 py-6">
      <header className="flex items-center gap-3">
        <img src="/icon.svg" alt="" className="size-9" />
        <div className="min-w-0 flex-1">
          <h1 className="text-xl font-semibold tracking-tight">Radar</h1>
          <p className="text-xs text-muted-foreground">
            {hub.inShell
              ? 'SWE, security and AI news'
              : 'SWE, security and AI news — open it from the hub for badges and notifications'}
          </p>
        </div>
        <nav className="flex rounded-lg bg-muted p-1">
          {views.map((v) => (
            <button
              key={v.id}
              onClick={() => setView(v.id)}
              aria-current={view === v.id ? 'page' : undefined}
              className={cn(
                'flex items-center gap-1.5 rounded-md px-3 py-1 text-sm font-medium text-muted-foreground',
                view === v.id && 'bg-background text-foreground shadow-xs',
              )}
            >
              <v.icon className="size-4" /> {v.name}
            </button>
          ))}
        </nav>
      </header>
      {view === 'news' ? (
        <News
          counts={counts}
          fetchStatus={fetchStatus}
          onChange={refreshSummary}
        />
      ) : (
        <Sources fetchStatus={fetchStatus} />
      )}
    </div>
  );
}
