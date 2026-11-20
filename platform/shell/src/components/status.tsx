import { cn } from '@hub/ui-kit';
import type { AppStatus } from '../api/types';

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
