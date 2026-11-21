// Small presentational pieces shared by Radar's screens.
import { cn } from '@hub/ui-kit';
import {
  BookOpen,
  Flag,
  FlaskConical,
  ListChecks,
  PlayCircle,
  Trophy,
  Wrench,
} from 'lucide-react';
import type * as React from 'react';
import { useEffect, useState, type ReactNode } from 'react';
import type { StepType, StepView } from './api';

export function ProgressBar({
  value,
  max,
  className,
}: {
  value: number;
  max: number;
  className?: string;
}) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  return (
    <div
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      className={cn('h-1.5 overflow-hidden rounded-full bg-muted', className)}
    >
      <div
        className="h-full rounded-full bg-primary transition-[width] duration-(--dur-slow) ease-(--ease-out)"
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

/** A progress ring, e.g. minutes this week against the goal. */
export function Ring({
  value,
  max,
  size = 56,
  children,
}: {
  value: number;
  max: number;
  size?: number;
  children?: ReactNode;
}) {
  const stroke = 6;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const pct = max > 0 ? Math.min(1, value / max) : 0;
  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90" aria-hidden="true">
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          strokeWidth={stroke}
          className="stroke-muted"
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - pct)}
          className={cn(
            'transition-[stroke-dashoffset] duration-(--dur-slow) ease-(--ease-out)',
            pct >= 1 ? 'stroke-success' : 'stroke-primary',
          )}
        />
      </svg>
      <div className="absolute inset-0 grid place-items-center text-xs font-semibold tabular-nums">
        {children}
      </div>
    </div>
  );
}

export interface SegmentOption<T extends string> {
  value: T;
  label: ReactNode;
  title?: string;
}

/** A row of mutually exclusive buttons. Clicking the active one clears it when `clearable`. */
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
  clearable = false,
  size = 'md',
}: {
  options: SegmentOption<T>[];
  value: T | '';
  onChange: (v: T | '') => void;
  label: string;
  clearable?: boolean;
  size?: 'sm' | 'md';
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="inline-flex rounded-lg bg-muted p-0.5"
    >
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={on}
            title={o.title}
            onClick={() => onChange(on && clearable ? '' : o.value)}
            className={cn(
              'rounded-md font-medium text-muted-foreground transition-colors duration-(--dur-fast) hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none',
              size === 'sm' ? 'px-2 py-1 text-xs' : 'px-3 py-1.5 text-sm',
              on && 'bg-background text-foreground shadow-xs',
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

const typeIcons: Record<string, typeof BookOpen> = {
  read: BookOpen,
  watch: PlayCircle,
  lab: FlaskConical,
  tool: Wrench,
  walkthrough: ListChecks,
  checkpoint: Flag,
  capstone: Trophy,
};

export function TypeIcon({
  type,
  kind,
  className,
}: {
  type: StepType | string;
  kind?: StepView['kind'];
  className?: string;
}) {
  const Icon =
    typeIcons[kind === 'capstone' ? 'capstone' : type] ?? typeIcons.read;
  return (
    <Icon className={cn('size-4 shrink-0', className)} aria-hidden="true" />
  );
}

export const typeLabel: Record<string, string> = {
  read: 'Read',
  watch: 'Watch',
  lab: 'Lab',
  tool: 'Tool',
  walkthrough: 'Walkthrough',
  checkpoint: 'Checkpoint',
};

export function Difficulty({ level }: { level: number }) {
  return (
    <span
      className="inline-flex items-center gap-0.5"
      title={`Difficulty ${level} of 5`}
      aria-label={`Difficulty ${level} of 5`}
    >
      {[1, 2, 3, 4, 5].map((i) => (
        <span
          key={i}
          className={cn(
            'size-1.5 rounded-full',
            i <= level ? 'bg-foreground/60' : 'bg-foreground/15',
          )}
        />
      ))}
    </span>
  );
}

export function minutes(n: number): string {
  if (n < 60) return `${n} min`;
  const h = Math.floor(n / 60);
  const m = n % 60;
  return m ? `${h} h ${m} min` : `${h} h`;
}

/** True only once `on` has stayed true for `ms`: skeletons for slow loads only. */
export function useDelayed(on: boolean, ms = 300): boolean {
  const [shown, setShown] = useState(false);
  useEffect(() => {
    if (!on) return;
    const t = setTimeout(() => setShown(true), ms);
    return () => {
      clearTimeout(t);
      setShown(false);
    };
  }, [on, ms]);
  return on && shown;
}

export function Skeleton({
  className,
  style,
}: {
  className?: string;
  style?: React.CSSProperties;
}) {
  return (
    <div
      style={style}
      className={cn(
        'animate-pulse rounded-md bg-muted motion-reduce:animate-none',
        className,
      )}
    />
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="rounded border border-border bg-muted px-1 font-mono text-[10px] text-muted-foreground">
      {children}
    </kbd>
  );
}

export function SectionTitle({
  children,
  action,
}: {
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="mb-3 flex items-baseline justify-between gap-3">
      <h2 className="text-sm font-semibold tracking-tight text-muted-foreground uppercase">
        {children}
      </h2>
      {action}
    </div>
  );
}
