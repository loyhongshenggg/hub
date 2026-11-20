import { cn, Tooltip, TooltipContent, TooltipTrigger } from '@hub/ui-kit';
import type { ReactNode } from 'react';
import { dockApps, useApps } from '../api/apps';
import type { AppInfo } from '../api/types';
import { useTabs } from '../state/tabs';
import { AppIcon } from './app-icon';
import { statusLabel, StatusDot } from './status';
import { ThemeToggle } from './theme-toggle';

function DockButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          aria-current={active ? 'page' : undefined}
          onClick={onClick}
          className={cn(
            'relative grid size-11 shrink-0 place-items-center rounded-xl outline-none transition-colors hover:bg-sidebar-accent/70 focus-visible:ring-[3px] focus-visible:ring-ring/50',
            active && 'bg-sidebar-accent',
          )}
        >
          {active && (
            <span className="absolute -left-2.5 h-5 w-1 rounded-r-full bg-primary" />
          )}
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

function DockApp({ app }: { app: AppInfo }) {
  const { active, openApp } = useTabs();
  return (
    <DockButton
      label={`${app.name} — ${statusLabel[app.status]}`}
      active={active === app.id}
      onClick={() => openApp(app.id)}
    >
      <AppIcon app={app} size={30} />
      <StatusDot
        status={app.status}
        className="absolute right-1 bottom-1 size-2.5 ring-2 ring-sidebar"
      />
    </DockButton>
  );
}

export function HubLogo({ size = 30 }: { size?: number }) {
  return (
    <svg viewBox="0 0 32 32" width={size} height={size} aria-hidden>
      <rect width="32" height="32" rx="8" className="fill-primary" />
      <g className="fill-primary-foreground">
        <rect x="7" y="7" width="8" height="8" rx="2" />
        <rect x="17" y="7" width="8" height="8" rx="2" opacity=".7" />
        <rect x="7" y="17" width="8" height="8" rx="2" opacity=".7" />
        <rect x="17" y="17" width="8" height="8" rx="2" />
      </g>
    </svg>
  );
}

/** Left rail: Home, pinned apps, then other apps that aren't off. */
export function Dock() {
  const { data } = useApps();
  const { active, activate } = useTabs();
  const { pinned, running } = dockApps(data?.apps ?? []);
  return (
    <nav
      aria-label="Dock"
      className="flex w-16 shrink-0 flex-col items-center border-r bg-sidebar py-3"
    >
      <DockButton
        label="Home"
        active={active === null}
        onClick={() => activate(null)}
      >
        <HubLogo />
      </DockButton>
      <div className="my-2.5 h-px w-8 bg-border" />
      <div className="flex min-h-0 w-full flex-1 flex-col items-center gap-1.5 overflow-y-auto">
        {pinned.map((a) => (
          <DockApp key={a.id} app={a} />
        ))}
        {pinned.length > 0 && running.length > 0 && (
          <div className="my-1 h-px w-6 bg-border" />
        )}
        {running.map((a) => (
          <DockApp key={a.id} app={a} />
        ))}
      </div>
      <ThemeToggle />
    </nav>
  );
}
