import type { HubClient } from '@hub/app-sdk';
import { Button, cn, TooltipProvider } from '@hub/ui-kit';
import { QueryClientProvider } from '@tanstack/react-query';
import {
  GraduationCap,
  History as HistoryIcon,
  Newspaper,
  Settings as SettingsIcon,
} from 'lucide-react';
import { ViewTransition, useCallback, useEffect, useState } from 'react';
import { api, type Counts, type FetchStatus } from './api';
import { Assess } from './assess';
import { History } from './history';
import { Home } from './home';
import { ItemSheetProvider } from './item-sheet';
import { Lesson } from './lesson';
import { News } from './news';
import { queryClient, useSaveSettings, useSettings } from './queries';
import { href, useRoute, type Route } from './router';
import { Settings } from './settings';
import { Sources } from './sources';
import { Track } from './track';

export function App({ hub }: { hub: HubClient }) {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <Shell hub={hub} />
      </TooltipProvider>
    </QueryClientProvider>
  );
}

const tabs: {
  route: Route;
  match: Route['name'][];
  name: string;
  icon: typeof Newspaper;
}[] = [
  {
    route: { name: 'home' },
    match: ['home', 'track', 'assess'],
    name: 'Learn',
    icon: GraduationCap,
  },
  {
    route: { name: 'news' },
    match: ['news', 'sources'],
    name: 'News',
    icon: Newspaper,
  },
  {
    route: { name: 'history' },
    match: ['history'],
    name: 'History',
    icon: HistoryIcon,
  },
];

function Shell({ hub }: { hub: HubClient }) {
  const route = useRoute();
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

  useAutoTimezone();

  const screenKey =
    route.name === 'lesson'
      ? `lesson/${route.track}/${route.chapter}`
      : route.name === 'track'
        ? `track/${route.track}`
        : route.name;
  const focus = route.name === 'lesson';

  return (
    <ItemSheetProvider onRead={refreshSummary}>
      {!focus && (
        <header className="sticky top-0 z-30 border-b bg-background/85 backdrop-blur supports-[backdrop-filter]:bg-background/70">
          <div className="@container mx-auto flex max-w-6xl items-center gap-3 px-4 py-2.5">
            <a
              href={href({ name: 'home' })}
              className="flex items-center gap-2.5"
              title={
                hub.inShell
                  ? undefined
                  : 'Open Radar from the hub for badges and notifications'
              }
            >
              <img src="/icon.svg" alt="" className="size-7" />
              <span className="font-semibold tracking-tight">Radar</span>
            </a>
            <nav className="ml-2 flex rounded-lg bg-muted p-0.5">
              {tabs.map((t) => {
                const on = t.match.includes(route.name);
                const badge =
                  t.name === 'News' && counts?.unread ? counts.unread : 0;
                return (
                  <a
                    key={t.name}
                    href={href(t.route)}
                    aria-current={on ? 'page' : undefined}
                    className={cn(
                      'flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm font-medium text-muted-foreground transition-colors duration-(--dur-fast) hover:text-foreground',
                      on && 'bg-background text-foreground shadow-xs',
                    )}
                  >
                    <t.icon className="size-4" />
                    <span className="hidden @md:inline">{t.name}</span>
                    {badge > 0 && (
                      <span className="rounded-full bg-primary/15 px-1.5 text-[10px] font-semibold text-primary tabular-nums">
                        {badge > 99 ? '99+' : badge}
                      </span>
                    )}
                  </a>
                );
              })}
            </nav>
            <div className="flex-1" />
            <Button
              asChild
              variant="ghost"
              size="icon-sm"
              aria-label="Settings"
              title="Settings"
            >
              <a href={href({ name: 'settings' })}>
                <SettingsIcon />
              </a>
            </Button>
          </div>
        </header>
      )}
      <ViewTransition
        key={screenKey}
        enter="vt-screen"
        exit="vt-screen"
        default="none"
      >
        <div className={cn(!focus && 'mx-auto max-w-6xl px-4 py-6 @container')}>
          <Screen
            route={route}
            counts={counts}
            fetchStatus={fetchStatus}
            onNewsChange={refreshSummary}
          />
        </div>
      </ViewTransition>
    </ItemSheetProvider>
  );
}

function Screen({
  route,
  counts,
  fetchStatus,
  onNewsChange,
}: {
  route: Route;
  counts: Counts | null;
  fetchStatus: FetchStatus | null;
  onNewsChange: () => void;
}) {
  switch (route.name) {
    case 'lesson':
      return (
        <Lesson track={route.track} chapter={route.chapter} step={route.step} />
      );
    case 'track':
      return <Track id={route.track} />;
    case 'news':
      return (
        <div className="mx-auto max-w-4xl">
          <News
            counts={counts}
            fetchStatus={fetchStatus}
            onChange={onNewsChange}
          />
        </div>
      );
    case 'sources':
      return (
        <div className="mx-auto max-w-4xl">
          <Sources fetchStatus={fetchStatus} />
        </div>
      );
    case 'history':
      return <History />;
    case 'settings':
      return <Settings />;
    case 'assess':
      return <Assess />;
    default:
      return <Home />;
  }
}

/** The weekly goal needs the learner's zone; take it from this device once. */
function useAutoTimezone() {
  const settings = useSettings();
  const save = useSaveSettings();
  const s = settings.data;
  const { mutate, isIdle } = save;
  useEffect(() => {
    if (s && !s.timezone && isIdle)
      mutate({
        ...s,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      });
  }, [s, isIdle, mutate]);
}
