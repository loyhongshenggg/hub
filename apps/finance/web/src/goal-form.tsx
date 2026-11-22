// The goal editor, shared by "fill in a goal", Claude drafts and editing.
import { Button, Input } from '@hub/ui-kit';
import { Plus, X } from 'lucide-react';
import { useState } from 'react';
import type { Goal, GoalKind, Milestone } from './api';
import { areaNames, kindNames, toCents, toDollars, today } from './format';
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

/**
 * An amount box. It keeps what was typed; the goal gets the amount in cents,
 * or NaN while the text isn't an amount (goalFormError then blocks saving).
 */
function MoneyInput({ cents, onChange, placeholder, label }: { cents: number | undefined; onChange: (c: number | undefined) => void; placeholder?: string; label?: string }) {
  const [text, setText] = useState(() => (cents ? toDollars(cents) : ''));
  const bad = text.trim() !== '' && !(toCents(text) >= 0);
  return (
    <div className="relative">
      <span className="pointer-events-none absolute inset-y-0 left-2.5 flex items-center text-sm text-muted-foreground">S$</span>
      <Input
        inputMode="decimal"
        className="tabular pl-8"
        placeholder={placeholder}
        aria-label={label}
        aria-invalid={bad || undefined}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          const t = e.target.value.trim();
          onChange(t === '' ? undefined : toCents(t) >= 0 ? toCents(t) : NaN);
        }}
      />
    </div>
  );
}

function Money({ label, cents, onChange, hint }: { label: string; cents: number; onChange: (c: number) => void; hint?: string }) {
  const bad = Number.isNaN(cents);
  return (
    <Field label={label} hint={bad ? <span className="text-destructive">Type an amount, e.g. 1700, 1,700.50 or 80k</span> : hint}>
      <MoneyInput cents={cents} onChange={(c) => onChange(c ?? 0)} />
    </Field>
  );
}

/** goalFormError is why a goal can't be saved yet, or null. */
export function goalFormError(g: GoalDraft): string | null {
  const k = g.kind;
  if (!g.title.trim()) return 'Add a title.';
  const amounts = [g.target, ...(k === 'target' || k === 'payoff' ? [g.startAmount, g.monthlyPlan] : []), ...(g.milestones ?? []).map((m) => m.amount)];
  if (amounts.some((a) => a !== undefined && Number.isNaN(a))) return 'Fix the amount marked in red.';
  if (k === 'payoff' ? !((g.startAmount ?? 0) > 0) : !((g.target ?? 0) > 0)) return k === 'payoff' ? "Set the balance you're paying off." : 'Set a target amount.';
  if ((k === 'target' || k === 'payoff') && g.targetDate && g.targetDate <= (g.startDate || today())) return 'The target date must be after the start date.';
  if ((g.milestones ?? []).some((m) => !m.label.trim() && (m.amount || m.date))) return 'Give each milestone a name.';
  return null;
}

let rowSeq = 0; // milestone row keys

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
  // Stable row keys, so removing a row never shows its amount in the next one.
  const [rowKeys, setRowKeys] = useState<number[]>(() => ms.map(() => rowSeq++));
  if (rowKeys.length !== ms.length) setRowKeys(ms.map((_, i) => rowKeys[i] ?? rowSeq++));

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
            <Field label="At most (times)" hint={Number.isNaN(value.target) ? <span className="text-destructive">Type a whole number</span> : undefined}>
              <Input
                inputMode="numeric"
                defaultValue={value.target || ''}
                onChange={(e) => {
                  const t = e.target.value.trim();
                  set({ target: t === '' ? 0 : /^\d+$/.test(t) ? Number(t) : NaN });
                }}
              />
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
              <div key={rowKeys[i] ?? i} className="grid grid-cols-[1fr_7rem_9rem_auto] gap-2 max-[520px]:grid-cols-2">
                <Input value={m.label} placeholder="Label" aria-label="Milestone name" onChange={(e) => setMs(i, { label: e.target.value })} />
                <MoneyInput cents={m.amount} placeholder="amount" label="Milestone amount" onChange={(c) => setMs(i, { amount: c === 0 ? undefined : c })} />
                <Input type="date" aria-label="Milestone date" value={m.date ?? ''} onChange={(e) => setMs(i, { date: e.target.value || undefined })} />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Remove milestone"
                  onClick={() => {
                    setRowKeys(rowKeys.filter((_, j) => j !== i));
                    set({ milestones: ms.filter((_, j) => j !== i) });
                  }}
                >
                  <X className="size-4" />
                </Button>
              </div>
            ))}
            {ms.some((m) => Number.isNaN(m.amount)) && <span className="text-xs text-destructive">Type each amount like 1700, 1,700.50 or 80k.</span>}
            <Button variant="outline" size="sm" className="self-start" onClick={() => {
                setRowKeys([...rowKeys, rowSeq++]);
                set({ milestones: [...ms, { label: '' }] });
              }}>
              <Plus className="size-4" /> Milestone
            </Button>
          </div>
        </Field>
      )}
    </div>
  );
}
