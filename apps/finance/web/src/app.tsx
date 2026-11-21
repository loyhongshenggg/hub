import type { HubClient } from '@hub/app-sdk';
import { cn, TooltipProvider } from '@hub/ui-kit';
import { QueryClientProvider } from '@tanstack/react-query';
import { FileUp, LayoutDashboard, ListOrdered, Settings as SettingsIcon, Target } from 'lucide-react';
import { useEffect } from 'react';
import { GoalsPage } from './goals';
import { ImportPage } from './import';
import { OverviewPage } from './overview';
import { queryClient, useStatus } from './queries';
import { href, useRoute, type Page } from './router';
import { SettingsPage } from './settings';
import { TransactionsPage } from './transactions';

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
  const route = useRoute();
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
        {route.page === 'overview' && <OverviewPage route={route} />}
        {route.page === 'goals' && <GoalsPage route={route} />}
        {route.page === 'transactions' && <TransactionsPage route={route} />}
        {route.page === 'import' && <ImportPage route={route} />}
        {route.page === 'settings' && <SettingsPage />}
      </main>
    </div>
  );
}
