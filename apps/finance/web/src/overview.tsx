import { Button, cn, Switch } from '@hub/ui-kit';
import {
  ArrowDownRight,
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  FileUp,
  Info,
  Lightbulb,
  Repeat,
  Sparkles,
  Target,
  TriangleAlert,
  ThumbsUp,
} from 'lucide-react';
import { useState } from 'react';
import type { Insight } from './api';
import { PaceLine, RankedBars, TrendColumns } from './charts';
import { date, money, monthName, plural } from './format';
import { useCategories, useGoals, useOverview } from './queries';
import { go, href, type Route } from './router';
import { SpendSheet, type SpendScope } from './sheets';
import { OwedCard } from './split';
import { Meter, Money, Section, StatusChip, Swatch } from './ui';

export function OverviewPage({ route }: { route: Route }) {
  const month = route.q.get('month') ?? undefined;
  const spread = route.q.get('spread') === '1';
  const { data: ov, isLoading } = useOverview(month, spread);
  const cats = useCategories();
  const [scope, setScope] = useState<SpendScope | null>(null);
  const [allInsights, setAllInsights] = useState(false);

  if (isLoading || !ov) return <OverviewSkeleton />;
  if (ov.months.length === 0) return <FirstRun />;

  const m = ov.month;
  const idx = ov.months.indexOf(m);
  const older = idx >= 0 ? ov.months[idx + 1] : undefined;
  const newer = idx > 0 ? ov.months[idx - 1] : undefined;
  const setMonth = (mm: string) => go('overview', undefined, { month: mm, spread }, true);
  const usual = ov.avgSpent;
  const diff = ov.spent - usual;
  const dataEndDay = ov.dataEnd.startsWith(m) ? Number(ov.dataEnd.slice(8)) : 31;

  return (
    <div className="flex flex-col gap-5">
      {/* Month switcher */}
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="ghost" size="icon-sm" disabled={!older} onClick={() => older && setMonth(older)} aria-label="Previous month">
          <ChevronLeft className="size-4" />
        </Button>
        <select
          value={m}
          onChange={(e) => setMonth(e.target.value)}
          className="rounded-md bg-transparent px-1 text-lg font-semibold tracking-tight outline-none hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50"
          aria-label="Month"
        >
          {!ov.months.includes(m) && <option value={m}>{monthName(m)}</option>}
          {ov.months.map((mm) => (
            <option key={mm} value={mm}>
              {monthName(mm)}
            </option>
          ))}
        </select>
        <Button variant="ghost" size="icon-sm" disabled={!newer} onClick={() => newer && setMonth(newer)} aria-label="Next month">
          <ChevronRight className="size-4" />
        </Button>
        {ov.partial && ov.dataEnd.startsWith(m) && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">Data up to {date(ov.dataEnd)}</span>
        )}
        <label className="ml-auto flex items-center gap-2 text-xs text-muted-foreground" title="Spread charges marked one-off (e.g. yearly premiums) over 12 months in the trend">
          <Switch checked={spread} onCheckedChange={(v) => go('overview', undefined, { month, spread: v }, true)} />
          Spread one-offs
        </label>
      </div>

      {/* Hero */}
      <section className="grid gap-4 rounded-xl border bg-card p-5 shadow-xs sm:grid-cols-[1.3fr_2fr]">
        <div>
          <div className="text-sm text-muted-foreground">Spent in {monthName(m)}</div>
          <div className="tabular text-4xl font-semibold tracking-tight sm:text-5xl">{money(ov.spent, { whole: ov.spent >= 100000 })}</div>
          {ov.avgMonths > 0 ? (
            <div className={cn('mt-1 flex items-center gap-1 text-sm', diff <= 0 ? 'text-[var(--good)]' : 'text-foreground')}>
              {diff <= 0 ? <ArrowDownRight className="size-4" /> : <ArrowUpRight className="size-4 text-[var(--bad)]" />}
              <span>
                {money(Math.abs(diff), { whole: true })} {diff <= 0 ? 'below' : 'above'} your usual {money(usual, { whole: true })}
              </span>
            </div>
          ) : (
            <div className="mt-1 text-sm text-muted-foreground">Import more months to compare with your usual.</div>
          )}
        </div>
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Kpi label="Transactions" value={String(ov.count)} />
          <Kpi label="Refunds" value={money(ov.refunds)} good={ov.refunds > 0} />
          <Kpi label="Last month" value={money(ov.prevSpent, { whole: true })} />
          <Kpi label="Recurring / month" value={money(ov.recurringPerMonth)} />
        </dl>
      </section>

      <GoalsStrip />

      <div className="grid gap-5 lg:grid-cols-[1.25fr_1fr]">
        <Section title="Where it went" action={<span className="text-xs text-muted-foreground">Click a row for details</span>}>
          {ov.categories.length === 0 ? (
            <p className="text-sm text-muted-foreground">No spending this month.</p>
          ) : (
            <RankedBars
              total={ov.spent}
              onPick={(id) => setScope({ month: m, category: id })}
              rows={ov.categories.map((c) => {
                const meta = cats.get(c.id);
                const vs = c.avg > 0 ? c.amount - c.avg : 0;
                return {
                  id: c.id,
                  label: meta.name,
                  value: c.amount,
                  slot: meta.slot,
                  note:
                    c.avg > 0 && Math.abs(vs) >= 1000 ? (
                      <span className={vs > 0 ? 'text-foreground' : 'text-[var(--good)]'}>
                        {vs > 0 ? '+' : '−'}
                        {money(Math.abs(vs), { whole: true })} vs usual
                      </span>
                    ) : (
                      plural(c.count, 'item')
                    ),
                };
              })}
            />
          )}
        </Section>
        <Section title="Insights" action={<Lightbulb className="size-4 text-muted-foreground" aria-hidden />}>
          {ov.insights.length === 0 ? (
            <p className="text-sm text-muted-foreground">Nothing stands out this month.</p>
          ) : (
            <>
              <ul className="flex flex-col gap-2">
                {(allInsights ? ov.insights : ov.insights.slice(0, 5)).map((i) => (
                  <InsightCard key={i.id} insight={i} onOpen={() => i.category && setScope({ month: m, category: i.category })} />
                ))}
              </ul>
              {ov.insights.length > 5 && (
                <Button variant="ghost" size="sm" className="mt-2" onClick={() => setAllInsights((v) => !v)}>
                  {allInsights ? 'Show fewer' : `Show ${ov.insights.length - 5} more`}
                </Button>
              )}
            </>
          )}
        </Section>
      </div>

      <div className="grid gap-5 lg:grid-cols-2">
        <Section title="Pace this month">
          <PaceLine pace={ov.pace} usual={ov.avgPace} dataEndDay={dataEndDay} monthLabel={monthName(m)} />
        </Section>
        <Section title="Last 12 months">
          <TrendColumns months={trimTrend(ov.trend)} groups={cats.groups} selected={m} onPick={setMonth} />
        </Section>
      </div>

      <div className="grid gap-5 lg:grid-cols-2">
        <Section title="Top merchants">
          <ul className="flex flex-col">
            {ov.merchants.map((mc) => {
              const meta = cats.get(mc.category);
              return (
                <li key={mc.name}>
                  <button
                    type="button"
                    onClick={() => setScope({ month: m, merchant: mc.name })}
                    className="flex w-full items-center gap-3 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent"
                  >
                    <Swatch slot={meta.slot} />
                    <span className="min-w-0 flex-1 truncate">{mc.name}</span>
                    <span className="text-xs text-muted-foreground">{plural(mc.count, 'time')}</span>
                    <Money cents={mc.amount} className="w-24 text-right" />
                  </button>
                </li>
              );
            })}
          </ul>
        </Section>
        <div className="flex flex-col gap-5">
        <OwedCard />
        <Section title="Recurring charges" action={<Repeat className="size-4 text-muted-foreground" aria-hidden />}>
          {ov.recurring.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              None detected yet. Charges show up here after they repeat at a steady amount (needs two or more months of data).
            </p>
          ) : (
            <ul className="flex flex-col">
              {ov.recurring.map((r) => (
                <li key={r.key + r.amount} className="flex items-center gap-3 px-2 py-1.5 text-sm">
                  <Swatch slot={cats.get(r.category).slot} />
                  <span className="min-w-0 flex-1 truncate">{r.name}</span>
                  <span className="text-xs text-muted-foreground">every {r.every} · last {date(r.last)}</span>
                  <Money cents={r.amount} className="w-20 text-right" />
                </li>
              ))}
            </ul>
          )}
        </Section>
        </div>
      </div>

      <SpendSheet scope={scope} onClose={() => setScope(null)} />
    </div>
  );
}

/** trimTrend drops empty leading months (keeps at least 6 columns). */
function trimTrend<T extends { hasData: boolean }>(months: T[]): T[] {
  const first = months.findIndex((x) => x.hasData);
  if (first < 0) return months.slice(-6);
  return months.slice(Math.min(first, Math.max(0, months.length - 6)));
}

function Kpi({ label, value, good }: { label: string; value: string; good?: boolean }) {
  return (
    <div className="rounded-lg bg-muted/60 px-3 py-2">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={cn('tabular text-lg font-semibold', good && 'text-[var(--good)]')}>{value}</dd>
    </div>
  );
}

const toneLook = {
  good: { icon: ThumbsUp, cls: 'text-[var(--good)]' },
  warn: { icon: TriangleAlert, cls: 'text-[var(--warn)]' },
  info: { icon: Info, cls: 'text-[var(--seq)]' },
} as const;

function InsightCard({ insight, onOpen }: { insight: Insight; onOpen: () => void }) {
  const t = toneLook[insight.tone];
  const Icon = t.icon;
  const clickable = !!insight.category;
  const inner = (
    <>
      <Icon className={cn('mt-0.5 size-4 shrink-0', t.cls)} aria-hidden />
      <span className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{insight.title}</span>
        <span className="text-xs text-muted-foreground">{insight.body}</span>
      </span>
    </>
  );
  return (
    <li className="fade-up">
      {clickable ? (
        <button type="button" onClick={onOpen} className="flex w-full gap-2.5 rounded-lg border px-3 py-2.5 text-left hover:bg-accent">
          {inner}
        </button>
      ) : (
        <div className="flex gap-2.5 rounded-lg border px-3 py-2.5">{inner}</div>
      )}
    </li>
  );
}

function GoalsStrip() {
  const { data: goals } = useGoals();
  const active = (goals ?? []).filter((g) => g.status === 'active');
  if (!goals) return null;
  if (active.length === 0) {
    return (
      <a href={href('goals')} className="flex items-center gap-3 rounded-xl border border-dashed px-4 py-3 text-sm hover:bg-accent">
        <Target className="size-5 text-[var(--seq)]" />
        <span className="flex-1">
          <span className="font-medium">Set your first goal.</span>{' '}
          <span className="text-muted-foreground">Describe it in your own words; it shows up here with its progress.</span>
        </span>
        <ChevronRight className="size-4 text-muted-foreground" />
      </a>
    );
  }
  return (
    <div className="flex gap-3 overflow-x-auto pb-1">
      {active.map((g) => (
        <a
          key={g.id}
          href={href('goals', g.id)}
          className="flex w-64 shrink-0 flex-col gap-2 rounded-xl border bg-card p-3 shadow-xs transition-colors hover:bg-accent"
        >
          <div className="flex items-start justify-between gap-2">
            <span className="line-clamp-1 text-sm font-medium">{g.title}</span>
            <StatusChip status={g.progress.status} />
          </div>
          <Meter
            value={g.kind === 'limit' ? Math.min(1, g.progress.percent) : g.progress.percent}
            tone={g.progress.status === 'over' || g.progress.status === 'behind' ? 'bad' : 'seq'}
            label={g.title}
          />
          <span className="line-clamp-1 text-xs text-muted-foreground">{g.progress.statusText}</span>
        </a>
      ))}
      <a href={href('goals')} className="flex w-32 shrink-0 items-center justify-center rounded-xl border border-dashed text-sm text-muted-foreground hover:bg-accent">
        All goals
      </a>
    </div>
  );
}

function FirstRun() {
  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-4 py-6">
      <h1 className="text-2xl font-semibold tracking-tight">Welcome to Finance</h1>
      <p className="text-muted-foreground">
        Two things get you going. Everything stays on this computer; Claude is optional and off until you switch it on.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        <a href={href('import')} className="flex flex-col gap-2 rounded-xl border bg-card p-4 shadow-xs hover:bg-accent">
          <FileUp className="size-6 text-[var(--seq)]" />
          <span className="font-medium">Import a card statement</span>
          <span className="text-sm text-muted-foreground">
            Drop the PDF. Every line is read, checked against the statement's totals, and sorted into categories.
          </span>
        </a>
        <a href={href('goals')} className="flex flex-col gap-2 rounded-xl border bg-card p-4 shadow-xs hover:bg-accent">
          <Sparkles className="size-6 text-[var(--seq)]" />
          <span className="font-medium">Describe a goal</span>
          <span className="text-sm text-muted-foreground">
            “Build a S$20k emergency fund by end-2027 and keep food delivery under S$250 a month.”
          </span>
        </a>
      </div>
    </div>
  );
}

function OverviewSkeleton() {
  return (
    <div className="flex animate-pulse flex-col gap-5" aria-busy>
      <div className="h-8 w-48 rounded bg-muted" />
      <div className="h-32 rounded-xl bg-muted" />
      <div className="grid gap-5 lg:grid-cols-2">
        <div className="h-64 rounded-xl bg-muted" />
        <div className="h-64 rounded-xl bg-muted" />
      </div>
    </div>
  );
}
