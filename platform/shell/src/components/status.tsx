import { cn } from '@hub/ui-kit';
import type { AppInfo, AppStatus } from '../api/types';

export const statusLabel: Record<AppStatus, string> = {
  running: 'Running',
  starting: 'Starting…',
  stopping: 'Stopping…',
  error: 'Error',
  stopped: 'Off',
};

const dot: Record<AppStatus, string> = {
  running: 'bg-success',
  starting: 'bg-warning animate-pulse',
  stopping: 'bg-warning animate-pulse',
  error: 'bg-destructive',
  stopped: 'bg-muted-foreground/35',
};

/** Small "DEV" tag shown while an app runs in dev mode. */
export function DevTag({ app }: { app: Pick<AppInfo, 'mode' | 'status'> }) {
  if (app.mode !== 'dev' || app.status === 'stopped') return null;
  return (
    <span
      title="Dev mode: host processes with live reload"
      className="rounded bg-warning/20 px-1 py-px font-mono text-[10px] font-semibold leading-none text-foreground"
    >
      DEV
    </span>
  );
}

export function StatusDot({
  status,
  className,
}: {
  status: AppStatus;
  className?: string;
}) {
  return (
    <span
      role="img"
      aria-label={statusLabel[status]}
      className={cn(
        'inline-block size-2 shrink-0 rounded-full',
        dot[status],
        className,
      )}
    />
  );
}
