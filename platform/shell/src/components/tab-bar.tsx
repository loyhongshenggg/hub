import { Badge, cn } from '@hub/ui-kit';
import { House, WifiOff, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { useApps } from '../api/apps';
import { useBridgeState } from '../state/bridge';
import { useTabs } from '../state/tabs';
import { AppIcon } from './app-icon';
import { BadgeCount } from './dock';
import { DevTag, StatusDot } from './status';

function Tab({
  active,
  onSelect,
  onClose,
  children,
}: {
  active: boolean;
  onSelect: () => void;
  onClose?: () => void;
  children: ReactNode;
}) {
  return (
    <div
      role="tab"
      tabIndex={0}
      aria-selected={active}
      onClick={onSelect}
      onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onSelect()}
      onAuxClick={(e) => e.button === 1 && onClose?.()}
      className={cn(
        'group relative -mb-px flex h-9 max-w-56 min-w-0 shrink-0 cursor-default select-none items-center gap-2 rounded-t-lg border border-transparent px-3 text-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
        active
          ? 'border-border border-b-background bg-background text-foreground'
          : 'text-muted-foreground hover:bg-sidebar-accent/60',
      )}
    >
      {children}
      {onClose && (
        <button
          type="button"
          aria-label="Close tab"
          onClick={(e) => {
            e.stopPropagation();
            onClose();
          }}
          className="-mr-1 grid size-5 place-items-center rounded opacity-0 hover:bg-accent group-hover:opacity-100 group-aria-selected:opacity-100"
        >
          <X className="size-3.5" />
        </button>
      )}
    </div>
  );
}

export function TabBar({ connected }: { connected: boolean }) {
  const { data } = useApps();
  const { open, active, activate, closeApp } = useTabs();
  const { badges, titles } = useBridgeState();
  return (
    <div
      role="tablist"
      aria-label="Open apps"
      className="flex h-11 shrink-0 items-end gap-1 overflow-x-auto border-b bg-sidebar px-2"
    >
      <Tab active={active === null} onSelect={() => activate(null)}>
        <House className="size-4" />
        Home
      </Tab>
      {open.map((id) => {
        const app = data?.apps.find((a) => a.id === id);
        if (!app) return null;
        return (
          <Tab
            key={id}
            active={active === id}
            onSelect={() => activate(id)}
            onClose={() => closeApp(id)}
          >
            <AppIcon app={app} size={16} />
            <span className="truncate" title={titles[id] ?? app.name}>
              {titles[id] ?? app.name}
            </span>
            <BadgeCount count={badges[id]} />
            <DevTag app={app} />
            <StatusDot status={app.status} />
          </Tab>
        );
      })}
      {!connected && (
        <Badge variant="warning" className="mb-2 ml-auto">
          <WifiOff /> Reconnecting to hub…
        </Badge>
      )}
    </div>
  );
}
