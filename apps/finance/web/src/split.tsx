// Splitting a cost you paid: everyone else's share is money owed to you,
// and only your share counts as your spending (Splitwise-style shares; the
// card amount itself never changes, so statements still reconcile).
import { Button, cn, Input, SheetContent, SheetDescription, SheetTitle } from '@hub/ui-kit';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, Loader2, Plus, Users, X } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Sheet } from './page';
import { api, type Person, type Share, type SplitMethod, type Txn } from './api';
import { date, money, plural, toCents, toDollars } from './format';
import { useInvalidateLedger } from './queries';
import { ErrorNote, Money, Section } from './ui';

interface Row {
  name: string;
  weight: string; // shares mode
  amount: string; // exact mode (dollars)
}

/** computeSplit mirrors api/internal/split: whole cents, largest remainder, ties to you. */
export function computeSplit(total: number, method: SplitMethod, myWeight: number, rows: { weight: number; amount: number }[]): { mine: number; others: number[] } | null {
  const sign = total < 0 ? -1 : 1;
  const abs = Math.abs(total);
  if (method === 'exact') {
    const sum = rows.reduce((s, r) => s + r.amount, 0);
    if (sum > abs || rows.some((r) => r.amount < 0)) return null;
    return { mine: (abs - sum) * sign, others: rows.map((r) => r.amount * sign) };
  }
  const weights = [method === 'shares' ? myWeight : 1, ...rows.map((r) => (method === 'shares' ? r.weight : 1))];
  const wsum = weights.reduce((s, w) => s + w, 0);
  if (!(wsum > 0) || weights.some((w) => w < 0 || !Number.isFinite(w))) return null;
  const exact = weights.map((w) => (abs * w) / wsum);
  const out = exact.map((x) => Math.floor(x));
  let given = out.reduce((s, x) => s + x, 0);
  const order = exact.map((x, i) => ({ i, f: x - Math.floor(x) })).sort((a, b) => b.f - a.f + 0 || a.i - b.i);
  for (let k = 0; given < abs; k++, given++) out[order[k % order.length].i]++;
  return { mine: out[0] * sign, others: out.slice(1).map((x) => x * sign) };
}

const methods: { id: SplitMethod; label: string; hint: string }[] = [
  { id: 'equal', label: 'Equally', hint: 'Everyone pays the same' },
  { id: 'shares', label: 'Unequally', hint: 'By shares, e.g. 2 : 1, or percentages' },
  { id: 'exact', label: 'Exact amounts', hint: 'Type each amount; you pay the rest' },
];

export function SplitEditor({ txn }: { txn: Txn }) {
  const qc = useQueryClient();
  const inv = useInvalidateLedger();
  const { data } = useQuery({ queryKey: ['split', txn.id], queryFn: () => api.split(txn.id) });
  const { data: peopleData } = useQuery({ queryKey: ['people'], queryFn: api.people });
  const shares = data?.shares ?? [];
  const isSplit = shares.length > 0;
  const [open, setOpen] = useState(false);
  const [method, setMethod] = useState<SplitMethod>((txn.splitMethod as SplitMethod) || 'equal');
  const [myWeight, setMyWeight] = useState(String(txn.myWeight || 1));
  const [rows, setRows] = useState<Row[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<unknown>(null);

  // Start editing from the saved split (or one empty row).
  const current: Row[] =
    rows ??
    (shares.length
      ? shares.map((s) => ({ name: s.name, weight: String(s.weight), amount: toDollars(Math.abs(s.amount)) }))
      : [{ name: '', weight: '1', amount: '' }]);
  const setRow = (i: number, p: Partial<Row>) => setRows(current.map((r, j) => (j === i ? { ...r, ...p } : r)));
  const parsed = current.map((r) => ({ weight: parseFloat(r.weight) || 0, amount: r.amount ? toCents(r.amount) || 0 : 0 }));
  const preview = computeSplit(txn.amount, method, parseFloat(myWeight) || 0, parsed);
  const known = (peopleData?.people ?? []).filter((p) => !current.some((r) => r.name.trim().toLowerCase() === p.name.toLowerCase()));

  // Exact mode: typing your own share fills in the last person's amount.
  const setMine = (dollars: string) => {
    const mine = dollars ? toCents(dollars) || 0 : 0;
    const others = parsed.slice(0, -1).reduce((s, r) => s + r.amount, 0);
    const rest = Math.max(0, Math.abs(txn.amount) - mine - others);
    setRows(current.map((r, j) => (j === current.length - 1 ? { ...r, amount: toDollars(rest) || '0' } : r)));
  };

  const save = async () => {
    setErr(null);
    setBusy(true);
    try {
      const parts = current
        .filter((r) => r.name.trim())
        .map((r) => ({
          name: r.name.trim(),
          weight: method === 'shares' ? parseFloat(r.weight) || 0 : undefined,
          amount: method === 'exact' ? toCents(r.amount || '0') || 0 : undefined,
        }));
      const v = await api.setSplit(txn.id, { method, myWeight: parseFloat(myWeight) || 0, parts });
      qc.setQueryData(['split', txn.id], v);
      void qc.invalidateQueries({ queryKey: ['people'] });
      inv();
      setRows(null);
      setOpen(false);
    } catch (e) {
      setErr(e);
    } finally {
      setBusy(false);
    }
  };

  const clear = async () => {
    setBusy(true);
    try {
      const v = await api.clearSplit(txn.id);
      qc.setQueryData(['split', txn.id], v);
      void qc.invalidateQueries({ queryKey: ['people'] });
      inv();
      setRows(null);
    } finally {
      setBusy(false);
    }
  };

  const toggleSettled = async (s: Share) => {
    await api.settleShare(s.id, !s.settledAt);
    void qc.invalidateQueries({ queryKey: ['split', txn.id] });
    void qc.invalidateQueries({ queryKey: ['people'] });
    void qc.invalidateQueries({ queryKey: ['status'] });
  };

  if (txn.kind === 'payment' || txn.kind === 'transfer') return null;

  return (
    <div className="rounded-lg border">
      <button type="button" onClick={() => setOpen((o) => !o)} className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm">
        <Users className="size-4 text-muted-foreground" />
        <span className="flex-1 font-medium">{isSplit ? 'Shared cost' : 'Split this cost'}</span>
        {isSplit ? (
          <span className="text-xs text-muted-foreground">
            your share <Money cents={txn.myAmount} /> of <Money cents={txn.amount} />
          </span>
        ) : (
          <span className="text-xs text-muted-foreground">paid for others too?</span>
        )}
      </button>

      {isSplit && !open && (
        <ul className="divide-y border-t text-sm">
          {shares.map((s) => (
            <li key={s.id} className="flex items-center gap-2 px-3 py-1.5">
              <span className="flex-1">{s.name}</span>
              <Money cents={s.amount} />
              <button
                type="button"
                onClick={() => void toggleSettled(s)}
                className={cn(
                  'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs',
                  s.settledAt ? 'border-[var(--good)] text-[var(--good)]' : 'text-muted-foreground hover:bg-accent',
                )}
                aria-pressed={!!s.settledAt}
              >
                {s.settledAt ? <Check className="size-3" /> : null}
                {s.settledAt ? 'Paid back' : 'Owes you'}
              </button>
            </li>
          ))}
          <li className="flex gap-2 px-3 py-2">
            <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
              Edit split
            </Button>
            <Button size="sm" variant="ghost" onClick={() => void clear()} disabled={busy}>
              Remove split
            </Button>
          </li>
        </ul>
      )}

      {open && (
        <div className="flex flex-col gap-3 border-t p-3">
          <div className="grid grid-cols-3 gap-1 rounded-md bg-muted p-1" role="radiogroup" aria-label="How to split">
            {methods.map((m) => (
              <button
                key={m.id}
                type="button"
                role="radio"
                aria-checked={method === m.id}
                title={m.hint}
                onClick={() => setMethod(m.id)}
                className={cn('rounded px-2 py-1 text-xs', method === m.id ? 'bg-background font-medium shadow-xs' : 'text-muted-foreground')}
              >
                {m.label}
              </button>
            ))}
          </div>
          <p className="-mt-1 text-xs text-muted-foreground">{methods.find((m) => m.id === method)?.hint}</p>

          <ul className="flex flex-col gap-2">
            <li className="grid grid-cols-[1fr_6.5rem_6rem] items-center gap-2 text-sm">
              <span className="font-medium">You</span>
              {method === 'shares' ? (
                <Input className="h-8" inputMode="decimal" value={myWeight} onChange={(e) => setMyWeight(e.target.value)} aria-label="Your share" />
              ) : method === 'exact' ? (
                <Input
                  className="h-8"
                  inputMode="decimal"
                  placeholder="My share"
                  value={preview ? toDollars(Math.abs(preview.mine)) || '0' : ''}
                  onChange={(e) => setMine(e.target.value)}
                  aria-label="Your share in dollars"
                />
              ) : (
                <span />
              )}
              <Money cents={preview?.mine ?? 0} className="text-right font-medium" />
            </li>
            {current.map((r, i) => (
              <li key={i} className="grid grid-cols-[1fr_6.5rem_6rem_auto] items-center gap-2 text-sm">
                <Input
                  className="h-8"
                  placeholder="Name (or “Others”)"
                  list="split-people"
                  value={r.name}
                  onChange={(e) => setRow(i, { name: e.target.value })}
                  aria-label={`Person ${i + 1}`}
                />
                {method === 'shares' ? (
                  <Input className="h-8" inputMode="decimal" value={r.weight} onChange={(e) => setRow(i, { weight: e.target.value })} aria-label={`${r.name || 'Person'} share`} />
                ) : method === 'exact' ? (
                  <Input className="h-8" inputMode="decimal" placeholder="S$" value={r.amount} onChange={(e) => setRow(i, { amount: e.target.value })} aria-label={`${r.name || 'Person'} amount`} />
                ) : (
                  <span />
                )}
                <Money cents={preview?.others[i] ?? 0} className="text-right text-muted-foreground" />
                <Button variant="ghost" size="icon-xs" aria-label="Remove person" onClick={() => setRows(current.filter((_, j) => j !== i))} disabled={current.length === 1}>
                  <X className="size-3" />
                </Button>
              </li>
            ))}
          </ul>
          <datalist id="split-people">
            {(peopleData?.people ?? []).map((p) => (
              <option key={p.id} value={p.name} />
            ))}
          </datalist>
          <div className="flex flex-wrap items-center gap-1.5">
            <Button size="sm" variant="outline" onClick={() => setRows([...current, { name: '', weight: '1', amount: '' }])}>
              <Plus className="size-3.5" /> Person
            </Button>
            {known.slice(0, 6).map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => {
                  const empty = current.findIndex((r) => !r.name.trim());
                  if (empty >= 0) setRow(empty, { name: p.name });
                  else setRows([...current, { name: p.name, weight: '1', amount: '' }]);
                }}
                className="rounded-full border px-2 py-0.5 text-xs hover:bg-accent"
              >
                + {p.name}
              </button>
            ))}
          </div>
          {!preview && <p className="text-xs text-[var(--bad)]">The amounts don't fit the S${(Math.abs(txn.amount) / 100).toFixed(2)} charged.</p>}
          {preview && (
            <p className="text-xs text-muted-foreground">
              You paid {money(txn.amount)}; your spending becomes {money(preview.mine)} and {money(txn.amount - preview.mine)} is owed to you.
            </p>
          )}
          <ErrorNote error={err} />
          <div className="flex gap-2">
            <Button size="sm" onClick={() => void save()} disabled={busy || !preview || !current.some((r) => r.name.trim())}>
              {busy && <Loader2 className="size-3.5 animate-spin" />} Save split
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { setOpen(false); setRows(null); }}>
              Cancel
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

/** OwedCard: who still owes you, on the Overview. */
export function OwedCard() {
  const { data } = useQuery({ queryKey: ['people'], queryFn: api.people });
  const [person, setPerson] = useState<Person | null>(null);
  const owing = (data?.people ?? []).filter((p) => p.owed !== 0);
  if (!data || owing.length === 0) return null;
  return (
    <Section title={<span className="flex items-center gap-2"><Users className="size-4 text-[var(--seq)]" /> Owed to you</span>} action={<Money cents={data.owedToYou} className="font-semibold" />}>
      <ul className="flex flex-col">
        {owing.map((p) => (
          <li key={p.id}>
            <button type="button" onClick={() => setPerson(p)} className="flex w-full items-center gap-3 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent">
              <span className="flex size-6 items-center justify-center rounded-full bg-muted text-xs font-medium">{p.name.slice(0, 1).toUpperCase()}</span>
              <span className="flex-1">{p.name}</span>
              <span className="text-xs text-muted-foreground">{plural(p.count, 'shared cost')}</span>
              <Money cents={p.owed} className="w-24 text-right" />
            </button>
          </li>
        ))}
      </ul>
      <PersonSheet person={person} onClose={() => setPerson(null)} />
    </Section>
  );
}

export function PersonSheet({ person, onClose }: { person: Person | null; onClose: () => void }) {
  return (
    <Sheet open={!!person} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="overflow-y-auto p-5 sm:max-w-md">{person && <PersonDetail person={person} />}</SheetContent>
    </Sheet>
  );
}

function PersonDetail({ person }: { person: Person }) {
  const qc = useQueryClient();
  const { data: shares = [] } = useQuery({ queryKey: ['person', person.id], queryFn: () => api.personShares(person.id) });
  const owed = useMemo(() => shares.filter((s) => !s.settledAt).reduce((a, s) => a + s.amount, 0), [shares]);
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['person', person.id] });
    void qc.invalidateQueries({ queryKey: ['people'] });
    void qc.invalidateQueries({ queryKey: ['status'] });
    void qc.invalidateQueries({ queryKey: ['split'] });
  };
  return (
    <div className="flex flex-col gap-4">
      <div>
        <SheetTitle>{person.name}</SheetTitle>
        <SheetDescription>
          Owes you <Money cents={owed} /> · {plural(shares.length, 'shared cost')}
        </SheetDescription>
      </div>
      {owed !== 0 && (
        <Button
          className="self-start"
          size="sm"
          onClick={async () => {
            await api.settlePerson(person.id);
            refresh();
          }}
        >
          <Check className="size-4" /> Mark all paid back
        </Button>
      )}
      <ul className="divide-y rounded-lg border text-sm">
        {shares.map((s) => (
          <li key={s.id} className={cn('flex items-center gap-3 px-3 py-2', s.settledAt && 'text-muted-foreground')}>
            <span className="w-14 text-xs">{date(s.date ?? '')}</span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate">{s.merchant}</span>
              <span className="text-xs text-muted-foreground">of {money(s.total ?? 0)}</span>
            </span>
            <Money cents={s.amount} />
            <button
              type="button"
              onClick={async () => {
                await api.settleShare(s.id, !s.settledAt);
                refresh();
              }}
              className={cn('rounded-full border px-2 py-0.5 text-xs', s.settledAt ? 'border-[var(--good)] text-[var(--good)]' : 'hover:bg-accent')}
              aria-pressed={!!s.settledAt}
            >
              {s.settledAt ? 'Paid back' : 'Owes you'}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
