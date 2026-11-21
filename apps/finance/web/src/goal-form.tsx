// The goal editor, shared by "fill in a goal", Claude drafts and editing.
import { Button, Input } from '@hub/ui-kit';
import { Plus, X } from 'lucide-react';
import type { Goal, GoalKind, Milestone } from './api';
import { areaNames, kindNames, toCents, toDollars } from './format';
import { useCategories } from './queries';
import { Field, selectCls, textareaCls } from './ui';

export type GoalDraft = Partial<Goal> & { kind: GoalKind; title: string };

export const blankGoal = (kind: GoalKind = 'target'): GoalDraft => ({
  kind,
  title: '',
  why: '',
  area: kind === 'limit' ? 'lifestyle' : 'safety',
  target: 0,
  startAmount: 0,
  targetDate: '',
  period: 'month',
  metric: 'amount',
  filter: { categories: [], merchants: [], keywords: [] },
  monthlyPlan: 0,
  milestones: [],
});

const kindHelp: Record<GoalKind, string> = {
  target: 'Reach an amount — emergency fund, down payment, an investment pot.',
  payoff: 'Bring a balance down to zero — a loan or card debt.',
  limit: 'Keep spending under a cap per period. Tracks itself from your transactions.',
  habit: 'Put in at least an amount every period — e.g. invest monthly.',
};

function Money({ label, cents, onChange, hint }: { label: string; cents: number; onChange: (c: number) => void; hint?: string }) {
  return (
    <Field label={label} hint={hint}>
      <div className="relative">
        <span className="pointer-events-none absolute inset-y-0 left-2.5 flex items-center text-sm text-muted-foreground">S$</span>
        <Input
          inputMode="decimal"
          className="tabular pl-8"
          defaultValue={toDollars(cents)}
          onChange={(e) => onChange(e.target.value.trim() === '' ? 0 : toCents(e.target.value) || 0)}
        />
      </div>
    </Field>
  );
}

const list = (s: string) =>
  s
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean);

export function GoalForm({ value, onChange }: { value: GoalDraft; onChange: (g: GoalDraft) => void }) {
  const cats = useCategories();
  const set = (p: Partial<Goal>) => onChange({ ...value, ...p });
  const k = value.kind;
  const filter = value.filter ?? { categories: [], merchants: [], keywords: [] };
  const toggleCat = (id: string) => {
    const cur = filter.categories ?? [];
    set({ filter: { ...filter, categories: cur.includes(id) ? cur.filter((c) => c !== id) : [...cur, id] } });
  };
  const ms = value.milestones ?? [];
  const setMs = (i: number, p: Partial<Milestone>) => set({ milestones: ms.map((m, j) => (j === i ? { ...m, ...p } : m)) });

  return (
    <div className="grid gap-4">
      <div className="grid gap-2 sm:grid-cols-4" role="radiogroup" aria-label="Kind of goal">
        {(Object.keys(kindNames) as GoalKind[]).map((kk) => (
          <button
            key={kk}
            type="button"
            role="radio"
            aria-checked={k === kk}
            onClick={() => set({ kind: kk })}
            className={`rounded-lg border px-3 py-2 text-left text-sm transition-colors ${k === kk ? 'border-primary bg-primary/5 font-medium' : 'hover:bg-accent'}`}
          >
            {kindNames[kk]}
          </button>
        ))}
      </div>
      <p className="-mt-2 text-xs text-muted-foreground">{kindHelp[k]}</p>
      <div className="grid gap-3 sm:grid-cols-[2fr_1fr]">
        <Field label="Title">
          <Input value={value.title} onChange={(e) => set({ title: e.target.value })} placeholder="Emergency fund" />
        </Field>
        <Field label="Area of life">
          <select className={selectCls} value={value.area ?? 'other'} onChange={(e) => set({ area: e.target.value })}>
            {Object.entries(areaNames).map(([a, n]) => (
              <option key={a} value={a}>
                {n}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <Field label="Why it matters" hint="In your own words — it shows on the goal to keep you going.">
        <textarea className={textareaCls} rows={2} value={value.why ?? ''} onChange={(e) => set({ why: e.target.value })} />
      </Field>

      {k === 'target' && (
        <div className="grid gap-3 sm:grid-cols-3">
          <Money label="Target" cents={value.target ?? 0} onChange={(c) => set({ target: c })} />
          <Money label="Already have" cents={value.startAmount ?? 0} onChange={(c) => set({ startAmount: c })} />
          <Money label="Plan per month" cents={value.monthlyPlan ?? 0} onChange={(c) => set({ monthlyPlan: c })} hint="Optional" />
        </div>
      )}
      {k === 'payoff' && (
        <div className="grid gap-3 sm:grid-cols-2">
          <Money label="Balance today" cents={value.startAmount ?? 0} onChange={(c) => set({ startAmount: c })} />
          <Money label="Plan per month" cents={value.monthlyPlan ?? 0} onChange={(c) => set({ monthlyPlan: c })} hint="Optional" />
        </div>
      )}
      {(k === 'limit' || k === 'habit') && (
        <div className="grid gap-3 sm:grid-cols-3">
          {k === 'limit' && value.metric === 'count' ? (
            <Field label="At most (times)">
              <Input inputMode="numeric" defaultValue={value.target || ''} onChange={(e) => set({ target: parseInt(e.target.value, 10) || 0 })} />
            </Field>
          ) : (
            <Money label={k === 'limit' ? 'At most' : 'At least'} cents={value.target ?? 0} onChange={(c) => set({ target: c })} />
          )}
          <Field label="Per">
            <select className={selectCls} value={value.period ?? 'month'} onChange={(e) => set({ period: e.target.value as Goal['period'] })}>
              <option value="week">Week</option>
              <option value="month">Month</option>
              <option value="year">Year</option>
            </select>
          </Field>
          {k === 'limit' && (
            <Field label="Measure">
              <select className={selectCls} value={value.metric ?? 'amount'} onChange={(e) => set({ metric: e.target.value as Goal['metric'], target: 0 })}>
                <option value="amount">Amount spent</option>
                <option value="count">Number of purchases</option>
              </select>
            </Field>
          )}
        </div>
      )}
      {k === 'limit' && (
        <div className="grid gap-3">
          <Field label="Counts spending in" hint="Pick categories, and/or name merchants or words. Nothing picked = all spending.">
            <div className="flex flex-wrap gap-1.5">
              {cats.list.map((c) => {
                const on = (filter.categories ?? []).includes(c.id);
                return (
                  <button
                    key={c.id}
                    type="button"
                    aria-pressed={on}
                    onClick={() => toggleCat(c.id)}
                    className={`rounded-full border px-2.5 py-1 text-xs ${on ? 'border-primary bg-primary text-primary-foreground' : 'hover:bg-accent'}`}
                  >
                    {c.name}
                  </button>
                );
              })}
            </div>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Merchants" hint="Comma-separated, e.g. foodpanda, Grab">
              <Input defaultValue={(filter.merchants ?? []).join(', ')} onChange={(e) => set({ filter: { ...filter, merchants: list(e.target.value) } })} />
            </Field>
            <Field label="Words in the description" hint="Comma-separated">
              <Input defaultValue={(filter.keywords ?? []).join(', ')} onChange={(e) => set({ filter: { ...filter, keywords: list(e.target.value) } })} />
            </Field>
          </div>
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Start">
          <Input type="date" value={value.startDate ?? ''} onChange={(e) => set({ startDate: e.target.value })} />
        </Field>
        {(k === 'target' || k === 'payoff') && (
          <Field label="Target date" hint="Optional">
            <Input type="date" value={value.targetDate ?? ''} onChange={(e) => set({ targetDate: e.target.value })} />
          </Field>
        )}
      </div>
      {(k === 'target' || k === 'payoff') && (
        <Field label="Milestones" hint="Optional steps on the way; amount-based ones tick themselves.">
          <div className="flex flex-col gap-2">
            {ms.map((m, i) => (
              <div key={i} className="grid grid-cols-[1fr_7rem_9rem_auto] gap-2 max-[520px]:grid-cols-2">
                <Input value={m.label} placeholder="Label" onChange={(e) => setMs(i, { label: e.target.value })} />
                <Input
                  inputMode="decimal"
                  placeholder="S$"
                  defaultValue={m.amount ? toDollars(m.amount) : ''}
                  onChange={(e) => setMs(i, { amount: e.target.value ? toCents(e.target.value) || undefined : undefined })}
                />
                <Input type="date" value={m.date ?? ''} onChange={(e) => setMs(i, { date: e.target.value || undefined })} />
                <Button variant="ghost" size="icon-sm" aria-label="Remove milestone" onClick={() => set({ milestones: ms.filter((_, j) => j !== i) })}>
                  <X className="size-4" />
                </Button>
              </div>
            ))}
            <Button variant="outline" size="sm" className="self-start" onClick={() => set({ milestones: [...ms, { label: '' }] })}>
              <Plus className="size-4" /> Milestone
            </Button>
          </div>
        </Field>
      )}
    </div>
  );
}
