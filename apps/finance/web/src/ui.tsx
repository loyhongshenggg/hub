// Small shared pieces.
import { cn } from '@hub/ui-kit';
import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  Clock,
  Flame,
  PauseCircle,
  TrendingUp,
  type LucideIcon,
} from 'lucide-react';
import type { ReactNode } from 'react';
import type { ProgressStatus } from './api';
import { money } from './format';

export function Money({ cents, className, sign, whole }: { cents: number; className?: string; sign?: boolean; whole?: boolean }) {
  return (
    <span className={cn('tabular whitespace-nowrap', cents < 0 && 'text-[var(--good)]', className)}>
      {money(cents, { sign, whole })}
    </span>
  );
}

/** Swatch for a category group colour slot (identity, beside text). */
export function Swatch({ slot, className }: { slot: number; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn('inline-block size-2.5 shrink-0 rounded-[3px]', className)}
      style={{ background: `var(--series-${slot})` }}
    />
  );
}

const statusLook: Record<ProgressStatus, { icon: LucideIcon; label: string; cls: string }> = {
  achieved: { icon: CheckCircle2, label: 'Achieved', cls: 'text-[var(--good)] bg-[var(--good)]/10' },
  ahead: { icon: TrendingUp, label: 'Ahead', cls: 'text-[var(--good)] bg-[var(--good)]/10' },
  on_track: { icon: CheckCircle2, label: 'On track', cls: 'text-[var(--good)] bg-[var(--good)]/10' },
  at_risk: { icon: Clock, label: 'At risk', cls: 'text-foreground bg-[var(--warn)]/20' },
  behind: { icon: AlertTriangle, label: 'Behind', cls: 'text-[var(--bad)] bg-[var(--bad)]/10' },
  over: { icon: Flame, label: 'Over', cls: 'text-[var(--bad)] bg-[var(--bad)]/10' },
  no_data: { icon: CircleDashed, label: 'Waiting for data', cls: 'text-muted-foreground bg-muted' },
  paused: { icon: PauseCircle, label: 'Paused', cls: 'text-muted-foreground bg-muted' },
};

/** Status with icon + label (never colour alone). */
export function StatusChip({ status, className }: { status: ProgressStatus; className?: string }) {
  const s = statusLook[status];
  const Icon = s.icon;
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium', s.cls, className)}>
      <Icon className="size-3.5" aria-hidden />
      {s.label}
    </span>
  );
}

/** Meter: one ratio against its limit, same-ramp track. */
export function Meter({ value, tone = 'seq', className, label }: { value: number; tone?: 'seq' | 'good' | 'warn' | 'bad'; className?: string; label?: string }) {
  const v = Math.max(0, Math.min(1, value));
  const color = tone === 'seq' ? 'var(--seq)' : `var(--${tone})`;
  return (
    <div
      role="meter"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(value * 100)}
      aria-label={label}
      className={cn('h-2 w-full overflow-hidden rounded-full bg-[var(--seq-track)]', className)}
    >
      <div className="grow-x h-full rounded-full" style={{ width: `${v * 100}%`, background: color }} />
    </div>
  );
}

export function Empty({ icon: Icon, title, children, className }: { icon: LucideIcon; title: string; children?: ReactNode; className?: string }) {
  return (
    <div className={cn('flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-10 text-center', className)}>
      <Icon className="size-8 text-muted-foreground" aria-hidden />
      <div className="font-medium">{title}</div>
      {children && <div className="max-w-md text-sm text-muted-foreground">{children}</div>}
    </div>
  );
}

export function Field({ label, hint, children, className }: { label: string; hint?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <label className={cn('flex flex-col gap-1.5 text-sm', className)}>
      <span className="font-medium">{label}</span>
      {children}
      {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
    </label>
  );
}

export const selectCls =
  'h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm shadow-xs outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30';

export const textareaCls =
  'w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30';

export function Section({ title, action, children, className }: { title: ReactNode; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn('rounded-xl border bg-card p-4 shadow-xs sm:p-5', className)}>
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="font-semibold tracking-tight">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  );
}

export function ErrorNote({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <div role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
      {error instanceof Error ? error.message : String(error)}
    </div>
  );
}
