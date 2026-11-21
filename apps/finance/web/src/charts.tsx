// Charts as plain SVG/HTML (docs/finance/design.md §8, dataviz method):
// thin marks, 4px rounded data ends on the baseline, 2px surface gaps,
// recessive grid, legend for ≥ 2 series, hover tooltips, a table view.
import { cn } from '@hub/ui-kit';
import { ChevronRight, Table2 } from 'lucide-react';
import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import type { DayPoint, GoalPoint, Group, MonthStat } from './api';
import { compact, date, money, monthName, monthShort } from './format';
import { Swatch } from './ui';

function useWidth<T extends HTMLElement>(): [React.RefObject<T | null>, number] {
  const ref = useRef<T>(null);
  const [w, setW] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setW(Math.floor(e.contentRect.width)));
    ro.observe(el);
    setW(Math.floor(el.getBoundingClientRect().width));
    return () => ro.disconnect();
  }, []);
  return [ref, w];
}

/** niceMax rounds an axis maximum up to 1/2/2.5/5 × 10^n. */
function niceMax(v: number): number {
  if (v <= 0) return 100;
  const p = 10 ** Math.floor(Math.log10(v));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p;
  return 10 * p;
}

// --- ranked bars (part-to-whole, one hue)

export interface BarRow {
  id: string;
  label: string;
  value: number;
  slot: number; // group swatch beside the name (identity)
  note?: ReactNode;
}

export function RankedBars({ rows, onPick, total }: { rows: BarRow[]; onPick?: (id: string) => void; total: number }) {
  const maxV = Math.max(1, ...rows.map((r) => r.value));
  return (
    <ul className="flex flex-col">
      {rows.map((r) => (
        <li key={r.id}>
          <button
            type="button"
            onClick={() => onPick?.(r.id)}
            className="group grid w-full grid-cols-[minmax(7rem,11rem)_1fr_auto] items-center gap-3 rounded-md px-2 py-1.5 text-left text-sm transition-colors duration-[var(--dur-fast)] hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none max-[480px]:grid-cols-[1fr_auto]"
          >
            <span className="flex min-w-0 items-center gap-2">
              <Swatch slot={r.slot} />
              <span className="truncate">{r.label}</span>
            </span>
            <span className="relative h-2.5 max-[480px]:col-span-2 max-[480px]:row-start-2">
              <span
                className="grow-x absolute inset-y-0 left-0 rounded-r-[4px] bg-[var(--seq)]"
                style={{ width: `${Math.max(0.5, (Math.max(0, r.value) / maxV) * 100)}%` }}
              />
            </span>
            <span className="flex items-center gap-1.5 text-right">
              <span className="flex flex-col items-end leading-tight">
                <span className="tabular font-medium">{money(r.value)}</span>
                <span className="tabular text-xs text-muted-foreground">
                  {total > 0 ? `${Math.round((r.value / total) * 100)}%` : ''}
                  {r.note ? <> · {r.note}</> : null}
                </span>
              </span>
              <ChevronRight className="size-4 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" aria-hidden />
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

// --- 12-month trend, stacked by group

export function TrendColumns({ months, groups, selected, onPick }: { months: MonthStat[]; groups: Group[]; selected: string; onPick: (m: string) => void }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const [table, setTable] = useState(false);
  const height = 200;
  const pad = { l: 44, r: 4, t: 8, b: 22 };
  const order = groups; // fixed slot order, bottom → top
  const maxV = niceMax(Math.max(...months.map((m) => m.total), 1));
  const plotW = Math.max(0, width - pad.l - pad.r);
  const plotH = height - pad.t - pad.b;
  const slotW = months.length ? plotW / months.length : 0;
  const barW = Math.max(4, Math.min(28, slotW * 0.62));
  const y = (v: number) => pad.t + plotH - (v / maxV) * plotH;
  const ticks = [0, 0.5, 1].map((f) => f * maxV);
  const present = order.filter((g) => months.some((m) => (m.byGroup[g.id] ?? 0) > 0));

  return (
    <div>
      <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        {present.map((g) => (
          <span key={g.id} className="inline-flex items-center gap-1.5">
            <Swatch slot={g.slot} />
            {g.name}
          </span>
        ))}
        <button
          type="button"
          onClick={() => setTable((t) => !t)}
          className="ml-auto inline-flex items-center gap-1 rounded px-1.5 py-0.5 hover:bg-accent hover:text-foreground"
          aria-pressed={table}
        >
          <Table2 className="size-3.5" aria-hidden /> {table ? 'Chart' : 'Table'}
        </button>
      </div>
      {table ? (
        <div className="max-h-64 overflow-auto rounded-md border">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-card text-xs text-muted-foreground">
              <tr>
                <th className="px-2 py-1.5 text-left font-medium">Month</th>
                {present.map((g) => (
                  <th key={g.id} className="px-2 py-1.5 text-right font-medium">{g.name}</th>
                ))}
                <th className="px-2 py-1.5 text-right font-medium">Total</th>
              </tr>
            </thead>
            <tbody>
              {months.filter((m) => m.hasData).reverse().map((m) => (
                <tr key={m.month} className="border-t">
                  <td className="px-2 py-1">{monthName(m.month, true)}</td>
                  {present.map((g) => (
                    <td key={g.id} className="tabular px-2 py-1 text-right">{money(m.byGroup[g.id] ?? 0, { whole: true })}</td>
                  ))}
                  <td className="tabular px-2 py-1 text-right font-medium">{money(m.total, { whole: true })}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
          {width > 0 && (
            <svg width={width} height={height} role="img" aria-label="Spending by month, stacked by group">
              {ticks.map((t) => (
                <g key={t}>
                  <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="var(--border)" strokeWidth={1} />
                  <text x={pad.l - 6} y={y(t)} dy="0.32em" textAnchor="end" className="fill-muted-foreground text-[10px]">
                    {compact(t)}
                  </text>
                </g>
              ))}
              {months.map((m, i) => {
                const cx = pad.l + slotW * (i + 0.5);
                let acc = 0;
                const segs = order
                  .map((g) => ({ g, v: Math.max(0, m.byGroup[g.id] ?? 0) }))
                  .filter((s) => s.v > 0);
                const sel = m.month === selected;
                return (
                  <g
                    key={m.month}
                    className="cursor-pointer"
                    onMouseEnter={() => setHover(i)}
                    onClick={() => m.hasData && onPick(m.month)}
                    opacity={hover !== null && hover !== i ? 0.55 : 1}
                  >
                    <rect x={cx - slotW / 2} y={pad.t} width={slotW} height={plotH} fill="transparent" />
                    {sel && <rect x={cx - slotW / 2 + 1} y={pad.t} width={slotW - 2} height={plotH} rx={4} fill="var(--accent)" />}
                    {segs.map((s, k) => {
                      const y0 = y(acc);
                      acc += s.v;
                      const y1 = y(acc);
                      const top = k === segs.length - 1;
                      const h = Math.max(0, y0 - y1 - (k > 0 ? 2 : 0)); // 2px surface gap
                      return top ? (
                        <path
                          key={s.g.id}
                          d={roundTop(cx - barW / 2, y1, barW, h, Math.min(4, h))}
                          fill={`var(--series-${s.g.slot})`}
                        />
                      ) : (
                        <rect key={s.g.id} x={cx - barW / 2} y={y0 - h} width={barW} height={h} fill={`var(--series-${s.g.slot})`} />
                      );
                    })}
                    <text
                      x={cx}
                      y={height - 6}
                      textAnchor="middle"
                      className={cn('text-[10px]', sel ? 'fill-foreground font-semibold' : 'fill-muted-foreground')}
                    >
                      {monthShort(m.month)}
                    </text>
                  </g>
                );
              })}
            </svg>
          )}
          {hover !== null && months[hover] && (
            <Tooltip x={pad.l + slotW * (hover + 0.5)} width={width}>
              <div className="mb-1 font-medium">{monthName(months[hover].month)}</div>
              {order
                .filter((g) => (months[hover].byGroup[g.id] ?? 0) !== 0)
                .reverse()
                .map((g) => (
                  <div key={g.id} className="flex items-center gap-2">
                    <Swatch slot={g.slot} />
                    <span className="flex-1 text-muted-foreground">{g.name}</span>
                    <span className="tabular">{money(months[hover].byGroup[g.id] ?? 0, { whole: true })}</span>
                  </div>
                ))}
              <div className="mt-1 flex justify-between border-t pt-1 font-medium">
                <span>Total</span>
                <span className="tabular">{money(months[hover].total, { whole: true })}</span>
              </div>
            </Tooltip>
          )}
        </div>
      )}
    </div>
  );
}

function roundTop(x: number, y: number, w: number, h: number, r: number): string {
  if (h <= 0) return '';
  r = Math.min(r, w / 2, h);
  return `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z`;
}

function Tooltip({ x, width, children }: { x: number; width: number; children: ReactNode }) {
  const left = Math.min(Math.max(8, x + 12), Math.max(8, width - 200));
  return (
    <div
      className="pointer-events-none absolute top-2 z-10 w-48 rounded-md border bg-popover px-2.5 py-2 text-xs shadow-md"
      style={{ left }}
    >
      {children}
    </div>
  );
}

// --- cumulative pace: this month vs usual

export function PaceLine({ pace, usual, dataEndDay, monthLabel }: { pace: DayPoint[]; usual: DayPoint[] | null; dataEndDay: number; monthLabel: string }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const height = 150;
  const pad = { l: 44, r: 8, t: 10, b: 20 };
  const shown = pace.filter((p) => p.day <= dataEndDay);
  const maxV = niceMax(Math.max(1, ...pace.map((p) => p.value), ...(usual ?? []).map((p) => p.value)));
  const n = pace.length || 30;
  const plotW = Math.max(0, width - pad.l - pad.r);
  const plotH = height - pad.t - pad.b;
  const x = (d: number) => pad.l + ((d - 1) / Math.max(1, n - 1)) * plotW;
  const y = (v: number) => pad.t + plotH - (v / maxV) * plotH;
  const path = (pts: DayPoint[]) => pts.map((p, i) => `${i ? 'L' : 'M'}${x(p.day)},${y(p.value)}`).join('');
  const onMove = (e: React.MouseEvent) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const d = Math.round(((e.clientX - r.left - pad.l) / Math.max(1, plotW)) * (n - 1)) + 1;
    setHover(d >= 1 && d <= n ? d : null);
  };
  return (
    <div>
      <div className="mb-2 flex items-center gap-3 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5">
          <span className="h-0.5 w-4 rounded bg-[var(--seq)]" /> {monthLabel}
        </span>
        {usual && (
          <span className="inline-flex items-center gap-1.5">
            <span className="h-0 w-4 border-t-2 border-dashed border-muted-foreground" /> Your usual month
          </span>
        )}
      </div>
      <div ref={ref} className="relative" onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
        {width > 0 && (
          <svg width={width} height={height} role="img" aria-label={`Cumulative spending in ${monthLabel} vs your usual month`}>
            {[0, 0.5, 1].map((f) => (
              <g key={f}>
                <line x1={pad.l} x2={width - pad.r} y1={y(f * maxV)} y2={y(f * maxV)} stroke="var(--border)" />
                <text x={pad.l - 6} y={y(f * maxV)} dy="0.32em" textAnchor="end" className="fill-muted-foreground text-[10px]">
                  {compact(f * maxV)}
                </text>
              </g>
            ))}
            {[1, 10, 20, n].map((d) => (
              <text key={d} x={x(d)} y={height - 4} textAnchor="middle" className="fill-muted-foreground text-[10px]">
                {d}
              </text>
            ))}
            {usual && <path d={path(usual)} fill="none" stroke="var(--muted-foreground)" strokeWidth={1.5} strokeDasharray="4 4" />}
            <path d={path(shown)} fill="none" stroke="var(--seq)" strokeWidth={2} strokeLinejoin="round" />
            {shown.length > 0 && (
              <circle cx={x(shown[shown.length - 1].day)} cy={y(shown[shown.length - 1].value)} r={4} fill="var(--seq)" stroke="var(--card)" strokeWidth={2} />
            )}
            {hover !== null && <line x1={x(hover)} x2={x(hover)} y1={pad.t} y2={pad.t + plotH} stroke="var(--muted-foreground)" strokeWidth={1} />}
          </svg>
        )}
        {hover !== null && (
          <Tooltip x={x(hover)} width={width}>
            <div className="mb-1 font-medium">Day {hover}</div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">{monthLabel}</span>
              <span className="tabular">{hover <= dataEndDay ? money(pace[hover - 1]?.value ?? 0, { whole: true }) : '—'}</span>
            </div>
            {usual && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Usual</span>
                <span className="tabular">{money(usual[hover - 1]?.value ?? 0, { whole: true })}</span>
              </div>
            )}
          </Tooltip>
        )}
      </div>
    </div>
  );
}

// --- goal history

/** PeriodBars: spend (or contributions) per period against a line at the target. */
export function PeriodBars({ points, target, count, mode }: { points: GoalPoint[]; target: number; count?: boolean; mode: 'limit' | 'habit' }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const height = 150;
  const pad = { l: 44, r: 6, t: 10, b: 20 };
  const fmt = (v: number) => (count ? String(v) : compact(v));
  const maxV = niceMax(Math.max(target * 1.15, ...points.map((p) => p.value), 1));
  const plotW = Math.max(0, width - pad.l - pad.r);
  const plotH = height - pad.t - pad.b;
  const slot = points.length ? plotW / points.length : 0;
  const bw = Math.max(6, Math.min(36, slot * 0.6));
  const y = (v: number) => pad.t + plotH - (v / maxV) * plotH;
  return (
    <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
      {width > 0 && (
        <svg width={width} height={height} role="img" aria-label="History by period">
          <line x1={pad.l} x2={width - pad.r} y1={y(0)} y2={y(0)} stroke="var(--border)" />
          <line x1={pad.l} x2={width - pad.r} y1={y(target)} y2={y(target)} stroke="var(--foreground)" strokeDasharray="3 3" strokeWidth={1} />
          <text x={pad.l - 6} y={y(target)} dy="0.32em" textAnchor="end" className="fill-foreground text-[10px]">
            {fmt(target)}
          </text>
          {points.map((p, i) => {
            const cx = pad.l + slot * (i + 0.5);
            const h = Math.max(0, y(0) - y(p.value));
            const bad = mode === 'limit' ? p.value > target : false;
            return (
              <g key={p.date} onMouseEnter={() => setHover(i)}>
                <rect x={cx - slot / 2} y={pad.t} width={slot} height={plotH} fill="transparent" />
                <path d={roundTop(cx - bw / 2, y(p.value), bw, h, 4)} fill={bad ? 'var(--bad)' : 'var(--seq)'} opacity={i === points.length - 1 ? 1 : 0.75} />
                <text x={cx} y={height - 5} textAnchor="middle" className="fill-muted-foreground text-[10px]">
                  {p.label.replace(/ \d{4}$/, '').slice(0, 3)}
                </text>
              </g>
            );
          })}
        </svg>
      )}
      {hover !== null && points[hover] && (
        <Tooltip x={pad.l + slot * (hover + 0.5)} width={width}>
          <div className="font-medium">{points[hover].label}</div>
          <div className="tabular">{count ? points[hover].value : money(points[hover].value)}</div>
          <div className="text-muted-foreground">
            {points[hover].met ? (mode === 'limit' ? 'Under the limit' : 'Done') : mode === 'limit' ? 'Over the limit' : 'Short'}
          </div>
        </Tooltip>
      )}
    </div>
  );
}

/** BalanceLine: check-ins over time toward a target (or down to zero). */
export function BalanceLine({ points, target, payoff }: { points: GoalPoint[]; target: number; payoff?: boolean }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const height = 150;
  const pad = { l: 48, r: 10, t: 12, b: 20 };
  const ts = points.map((p) => new Date(p.date).getTime());
  const t0 = Math.min(...ts);
  const t1 = Math.max(...ts, t0 + 86400000);
  const maxV = niceMax(Math.max(payoff ? 0 : target, ...points.map((p) => p.value), 1));
  const plotW = Math.max(0, width - pad.l - pad.r);
  const plotH = height - pad.t - pad.b;
  const x = (t: number) => pad.l + ((t - t0) / (t1 - t0)) * plotW;
  const y = (v: number) => pad.t + plotH - (v / maxV) * plotH;
  const d = points.map((p, i) => `${i ? 'L' : 'M'}${x(ts[i])},${y(p.value)}`).join('');
  return (
    <div ref={ref}>
      {width > 0 && (
        <svg width={width} height={height} role="img" aria-label="Progress over time">
          <line x1={pad.l} x2={width - pad.r} y1={y(0)} y2={y(0)} stroke="var(--border)" />
          {!payoff && target > 0 && (
            <>
              <line x1={pad.l} x2={width - pad.r} y1={y(target)} y2={y(target)} stroke="var(--foreground)" strokeDasharray="3 3" />
              <text x={pad.l - 6} y={y(target)} dy="0.32em" textAnchor="end" className="fill-foreground text-[10px]">
                {compact(target)}
              </text>
            </>
          )}
          <text x={pad.l - 6} y={y(0)} dy="0.32em" textAnchor="end" className="fill-muted-foreground text-[10px]">
            0
          </text>
          <path d={d} fill="none" stroke="var(--seq)" strokeWidth={2} strokeLinejoin="round" />
          {points.map((p, i) => (
            <circle key={i} cx={x(ts[i])} cy={y(p.value)} r={4} fill="var(--seq)" stroke="var(--card)" strokeWidth={2}>
              <title>{`${date(p.date, true)}: ${money(p.value)}`}</title>
            </circle>
          ))}
          <text x={pad.l} y={height - 4} className="fill-muted-foreground text-[10px]">
            {date(points[0]?.date ?? '', true)}
          </text>
          <text x={width - pad.r} y={height - 4} textAnchor="end" className="fill-muted-foreground text-[10px]">
            {date(points[points.length - 1]?.date ?? '', true)}
          </text>
        </svg>
      )}
    </div>
  );
}
