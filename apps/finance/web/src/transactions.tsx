import { Button, cn, Input, Sheet, SheetContent, SheetTitle } from '@hub/ui-kit';
import { AlertCircle, Download, Plus, Search, Target, Users, X } from 'lucide-react';
import { useDeferredValue, useEffect, useState } from 'react';
import { api, type Kind, type Txn, type TxnQuery } from './api';
import { money, monthName, plural, toCents, today, txnKindNames } from './format';
import { useCategories, useGoals, useInvalidateLedger, useOverview, useTxns } from './queries';
import { go, type Route } from './router';
import { TxnList, TxnSheet } from './sheets';
import { ErrorNote, Field, selectCls } from './ui';

/** The list query for a #/transactions route (also prefetched by app.tsx). */
export function txnQueryFor(q: URLSearchParams, search = q.get('q') ?? ''): TxnQuery {
  const month = q.get('month') ?? '';
  return {
    month,
    category: q.get('category') ?? '',
    kind: q.get('kind') ?? '',
    review: q.get('review') === '1',
    split: q.get('split') === '1',
    q: search,
    goal: Number(q.get('goal')) || undefined,
    limit: month ? undefined : 500,
  };
}

export function TransactionsPage({ route }: { route: Route }) {
  const q = route.q;
  const month = q.get('month') ?? '';
  const category = q.get('category') ?? '';
  const kind = q.get('kind') ?? '';
  const review = q.get('review') === '1';
  const shared = q.get('split') === '1';
  const goalId = Number(q.get('goal')) || undefined;
  const { data: goals } = useGoals();
  const goal = goalId ? goals?.find((g) => g.id === goalId) : undefined;
  const [search, setSearch] = useState(q.get('q') ?? '');
  const deferred = useDeferredValue(search);
  const cats = useCategories();
  const { data: ov } = useOverview();
  const { data: txns = noTxns, isFetching } = useTxns(txnQueryFor(q, deferred));
  const [edit, setEdit] = useState<Txn | null>(null);
  const [adding, setAdding] = useState(false);
  // The page stays mounted while hidden (app.tsx): close its sheets on leaving.
  useEffect(
    () => () => {
      setEdit(null);
      setAdding(false);
    },
    [],
  );
  const set = (p: Record<string, string | boolean | undefined>) =>
    go('transactions', undefined, { month, category, kind, review, split: shared, q: search, goal: goalId, ...p }, true);

  const out = txns.filter((t) => t.amount > 0 && !t.excluded && (t.kind === 'purchase' || t.kind === 'fee')).reduce((s, t) => s + t.myAmount, 0);
  const back = txns.filter((t) => t.amount < 0 && t.kind === 'refund').reduce((s, t) => s - t.amount, 0);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-xl font-semibold tracking-tight">Transactions</h1>
        <span className="text-sm text-muted-foreground">
          {plural(txns.length, 'entry', 'entries')} · {money(out)} your spending{back ? ` · ${money(back)} refunded` : ''}
        </span>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
            <Plus className="size-4" /> Add
          </Button>
          <Button variant="ghost" size="sm" asChild>
            <a href="/api/export.csv">
              <Download className="size-4" /> CSV
            </a>
          </Button>
        </div>
      </div>

      {goalId && (
        <div className="flex items-center gap-2 self-start rounded-full border border-primary/40 bg-primary/10 py-1 pr-1 pl-3 text-sm">
          <Target className="size-4 text-[var(--seq)]" aria-hidden />
          <span>
            Counted by <span className="font-medium">{goal?.title ?? 'a goal'}</span>
          </span>
          <button type="button" onClick={() => set({ goal: undefined })} className="rounded-full p-1 hover:bg-accent" aria-label="Show all transactions, not just this goal's">
            <X className="size-3.5" />
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-2 sm:grid-cols-[2fr_1fr_1fr_1fr_auto_auto]">
        <div className="relative col-span-2 sm:col-span-1">
          <Search className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
          <Input
            className="pl-8"
            placeholder="Search merchant, description, note"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onBlur={() => set({ q: search })}
            aria-label="Search"
          />
        </div>
        <select className={selectCls} value={month} onChange={(e) => set({ month: e.target.value })} aria-label="Month">
          <option value="">All months</option>
          {(ov?.months ?? []).map((m) => (
            <option key={m} value={m}>
              {monthName(m)}
            </option>
          ))}
        </select>
        <select className={selectCls} value={category} onChange={(e) => set({ category: e.target.value })} aria-label="Category">
          <option value="">All categories</option>
          {cats.list.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        <select className={selectCls} value={kind} onChange={(e) => set({ kind: e.target.value })} aria-label="Type">
          <option value="">All types</option>
          {Object.entries(txnKindNames).map(([k, n]) => (
            <option key={k} value={k}>
              {n}
            </option>
          ))}
        </select>
        <button
          type="button"
          aria-pressed={review}
          onClick={() => set({ review: !review })}
          className={cn(
            'flex h-9 items-center justify-center gap-1.5 rounded-md border px-3 text-sm whitespace-nowrap',
            review ? 'border-[var(--warn)] bg-[var(--warn)]/15' : 'hover:bg-accent',
          )}
        >
          <AlertCircle className="size-4" /> To check
        </button>
        <button
          type="button"
          aria-pressed={shared}
          onClick={() => set({ split: !shared })}
          className={cn('flex h-9 items-center justify-center gap-1.5 rounded-md border px-3 text-sm whitespace-nowrap', shared ? 'border-primary bg-primary/10' : 'hover:bg-accent')}
        >
          <Users className="size-4" /> Shared
        </button>
      </div>

      {review && txns.length > 0 && (
        <p className="text-sm text-muted-foreground">
          These were matched with low confidence, recovered from a second OCR pass, or (Grab) can't tell a ride from food. Open each one and press <b>Looks right</b> or
          fix it — fixing one can fix all similar ones.
        </p>
      )}

      <div className={cn('transition-opacity', isFetching && 'opacity-70')}>
        <TxnList txns={txns} onPick={setEdit} showCategory />
      </div>
      <TxnSheet txn={edit} onClose={() => setEdit(null)} />
      <AddSheet open={adding} onClose={() => setAdding(false)} />
    </div>
  );
}

const noTxns: Txn[] = [];

function AddSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const cats = useCategories();
  const inv = useInvalidateLedger();
  const [f, setF] = useState({ date: today(), amount: '', kind: 'purchase' as Kind, category: 'dining', merchant: '', note: '' });
  const [err, setErr] = useState<unknown>(null);
  const submit = async () => {
    setErr(null);
    try {
      await api.addTxn({ ...f, amount: toCents(f.amount) });
      inv();
      setF({ ...f, amount: '', merchant: '', note: '' });
      onClose();
    } catch (e) {
      setErr(e);
    }
  };
  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="overflow-y-auto p-5 sm:max-w-md">
        <div className="flex flex-col gap-4">
          <SheetTitle>Add a transaction</SheetTitle>
          <p className="text-sm text-muted-foreground">For cash, other cards, or income (salary) so savings-rate insights can use it.</p>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Date">
              <Input type="date" value={f.date} onChange={(e) => setF({ ...f, date: e.target.value })} />
            </Field>
            <Field label="Amount (S$)">
              <Input inputMode="decimal" value={f.amount} onChange={(e) => setF({ ...f, amount: e.target.value })} />
            </Field>
          </div>
          <Field label="Merchant / source">
            <Input value={f.merchant} onChange={(e) => setF({ ...f, merchant: e.target.value })} />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Type">
              <select className={selectCls} value={f.kind} onChange={(e) => setF({ ...f, kind: e.target.value as Kind })}>
                {Object.entries(txnKindNames).map(([k, n]) => (
                  <option key={k} value={k}>
                    {n}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Category">
              <select className={selectCls} value={f.category} onChange={(e) => setF({ ...f, category: e.target.value })}>
                {cats.list.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </Field>
          </div>
          <Field label="Note">
            <Input value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} />
          </Field>
          <ErrorNote error={err} />
          <Button onClick={() => void submit()} disabled={!f.merchant || !(toCents(f.amount) > 0)}>
            Add
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
