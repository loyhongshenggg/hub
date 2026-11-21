// Side sheets: spending detail (category or merchant → entries) and the
// transaction editor.
import { Button, Input, Sheet, SheetContent, SheetDescription, SheetTitle, Switch } from '@hub/ui-kit';
import { AlertCircle, Globe, Pencil, PenLine, RotateCcw, Trash2, Users } from 'lucide-react';
import { useState } from 'react';
import { api, type Kind, type Txn } from './api';
import { RankedBars } from './charts';
import { date, money, monthName, plural, toCents, toDollars, txnKindNames } from './format';
import { useCategories, useInvalidateLedger, usePatchTxn, useTxns } from './queries';
import { href } from './router';
import { SplitEditor } from './split';
import { ErrorNote, Field, Money, selectCls, Swatch, textareaCls } from './ui';

export interface SpendScope {
  month: string;
  category?: string;
  merchant?: string;
}

export function SpendSheet({ scope, onClose }: { scope: SpendScope | null; onClose: () => void }) {
  const [edit, setEdit] = useState<Txn | null>(null);
  return (
    <>
      <Sheet open={!!scope} onOpenChange={(o) => !o && onClose()}>
        <SheetContent className="overflow-y-auto p-5 sm:max-w-xl">{scope && <SpendDetail scope={scope} onEdit={setEdit} />}</SheetContent>
      </Sheet>
      <TxnSheet txn={edit} onClose={() => setEdit(null)} />
    </>
  );
}

function SpendDetail({ scope, onEdit }: { scope: SpendScope; onEdit: (t: Txn) => void }) {
  const cats = useCategories();
  const { data: txns = [] } = useTxns({ month: scope.month, category: scope.category, merchant: scope.merchant, spending: true });
  const total = txns.reduce((s, t) => s + (t.myAmount ?? t.amount), 0);
  const title = scope.category ? cats.get(scope.category).name : (scope.merchant ?? '');
  const byMerchant = new Map<string, { value: number; count: number; cat: string }>();
  for (const t of txns) {
    const k = t.merchant || t.raw;
    const e = byMerchant.get(k) ?? { value: 0, count: 0, cat: t.category };
    e.value += t.myAmount ?? t.amount;
    if (t.amount > 0) e.count++;
    byMerchant.set(k, e);
  }
  const rows = [...byMerchant.entries()]
    .sort((a, b) => b[1].value - a[1].value)
    .map(([name, e]) => ({ id: name, label: name, value: e.value, slot: cats.get(e.cat).slot, note: plural(e.count, 'time') }));

  return (
    <div className="flex flex-col gap-5">
      <div>
        <SheetTitle className="flex items-center gap-2 text-lg">
          {scope.category && <Swatch slot={cats.get(scope.category).slot} className="size-3" />}
          {title}
        </SheetTitle>
        <SheetDescription>
          {monthName(scope.month)} · {plural(txns.filter((t) => t.amount > 0).length, 'charge')} · <Money cents={total} />
        </SheetDescription>
      </div>
      {scope.category && rows.length > 1 && (
        <div>
          <h3 className="mb-1 text-sm font-medium">By merchant</h3>
          <RankedBars rows={rows} total={total} />
        </div>
      )}
      <div>
        <h3 className="mb-1 text-sm font-medium">Entries</h3>
        <TxnList txns={txns} onPick={onEdit} />
      </div>
      <a
        className="text-sm text-primary underline-offset-4 hover:underline"
        href={href('transactions', undefined, { month: scope.month, category: scope.category, q: scope.merchant })}
      >
        Open in Transactions
      </a>
    </div>
  );
}

export function TxnList({ txns, onPick, showCategory }: { txns: Txn[]; onPick: (t: Txn) => void; showCategory?: boolean }) {
  const cats = useCategories();
  if (txns.length === 0) return <p className="py-4 text-sm text-muted-foreground">Nothing here.</p>;
  return (
    <ul className="divide-y rounded-lg border">
      {txns.map((t) => {
        const cat = cats.get(t.category);
        return (
          <li key={t.id}>
            <button type="button" onClick={() => onPick(t)} className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-accent">
              <span className="tabular w-14 shrink-0 text-xs text-muted-foreground">{date(t.date)}</span>
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="flex items-center gap-1.5">
                  <span className="truncate font-medium">{t.merchant || t.raw}</span>
                  {t.needsReview && <AlertCircle className="size-3.5 shrink-0 text-[var(--warn)]" aria-label="Check this one" />}
                  {t.foreign && <Globe className="size-3.5 shrink-0 text-muted-foreground" aria-label="Foreign" />}
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  {showCategory && t.category ? (
                    <span className="mr-1.5 inline-flex items-center gap-1">
                      <Swatch slot={cat.slot} className="size-2" />
                      {cat.name} ·
                    </span>
                  ) : null}
                  {t.kind !== 'purchase' ? `${txnKindNames[t.kind]} · ` : ''}
                  {t.raw}
                </span>
              </span>
              <span className="flex flex-col items-end">
                <Money cents={t.myAmount ?? t.amount} className={t.excluded ? 'text-muted-foreground line-through' : ''} />
                {t.myAmount !== t.amount ? (
                  <span className="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
                    {t.othersShare ? <Users className="size-3" aria-label="Shared" /> : <PenLine className="size-3" aria-label="Your cost, edited" />} of{' '}
                    {money(t.amount)}
                  </span>
                ) : null}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

/** Grab and similar descriptors cover different things (ride vs food). */
const ambiguousKeys = new Set(['grab', 'shopee', 'amazon']);

export function TxnSheet({ txn, onClose }: { txn: Txn | null; onClose: () => void }) {
  return (
    <Sheet open={!!txn} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="overflow-y-auto p-5 sm:max-w-md">{txn && <TxnEditor key={txn.id} txn={txn} onDone={onClose} />}</SheetContent>
    </Sheet>
  );
}

function TxnEditor({ txn, onDone }: { txn: Txn; onDone: () => void }) {
  const cats = useCategories();
  const patch = usePatchTxn();
  const inv = useInvalidateLedger();
  const [merchant, setMerchant] = useState(txn.merchant);
  const [category, setCategory] = useState(txn.category);
  const [kind, setKind] = useState<Kind>(txn.kind);
  const [note, setNote] = useState(txn.note);
  const [oneOff, setOneOff] = useState(txn.oneOff);
  const [excluded, setExcluded] = useState(txn.excluded);
  const manual = txn.source === 'manual';
  const [when, setWhen] = useState(txn.date);
  const [amount, setAmount] = useState(toDollars(Math.abs(txn.amount)));
  const shared = txn.othersShare !== 0;
  const [myCost, setMyCost] = useState(toDollars(Math.abs(txn.myAmount)) || '0');
  const [resetCost, setResetCost] = useState(false);
  const myCostCents = myCost.trim() === '' ? NaN : toCents(myCost);
  const costChanged = !shared && !resetCost && Number.isFinite(myCostCents) && myCostCents !== Math.abs(txn.myAmount);
  const costInvalid = !shared && (!Number.isFinite(myCostCents) || myCostCents < 0 || myCostCents > Math.abs(manual ? toCents(amount) || 0 : txn.amount));
  const [similar, setSimilar] = useState(!ambiguousKeys.has(txn.merchantKey) && txn.merchantKey !== 'transit');
  const [applied, setApplied] = useState<number | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const changedIdentity = merchant !== txn.merchant || category !== txn.category || kind !== txn.kind;

  const save = async (reviewedOnly = false) => {
    setErr(null);
    try {
      const res = await patch.mutateAsync({
        id: txn.id,
        patch: reviewedOnly
          ? { reviewed: true }
          : {
              merchant: merchant !== txn.merchant ? merchant : undefined,
              category: category !== txn.category ? category : undefined,
              kind: kind !== txn.kind ? kind : undefined,
              note: note !== txn.note ? note : undefined,
              oneOff: oneOff !== txn.oneOff ? oneOff : undefined,
              excluded: excluded !== txn.excluded ? excluded : undefined,
              date: manual && when !== txn.date ? when : undefined,
              amount: manual && toCents(amount) !== Math.abs(txn.amount) && toCents(amount) > 0 ? Math.sign(txn.amount || 1) * toCents(amount) : undefined,
              myCost: costChanged ? myCostCents : undefined,
              clearMyCost: resetCost && txn.myCost !== null ? true : undefined,
              reviewed: true,
              applyToSimilar: similar && changedIdentity,
            },
      });
      if (res.applied > 1) setApplied(res.applied);
      else onDone();
    } catch (e) {
      setErr(e);
    }
  };

  const remove = async () => {
    if (!window.confirm('Delete this transaction?')) return;
    try {
      await api.deleteTxn(txn.id);
      inv();
      onDone();
    } catch (e) {
      setErr(e);
    }
  };

  if (applied !== null) {
    return (
      <div className="flex flex-col gap-4 pt-6">
        <SheetTitle>Saved</SheetTitle>
        <p className="text-sm">
          Updated {applied} transactions from <span className="font-medium">{txn.raw.split(' ').slice(0, 3).join(' ')}</span>. Future imports
          will use this too (Settings → Your rules).
        </p>
        <Button onClick={onDone}>Done</Button>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <SheetTitle className="flex items-center gap-2 pr-8">
          <Pencil className="size-4 text-muted-foreground" />
          {txn.merchant || txn.raw}
        </SheetTitle>
        <SheetDescription>
          {date(txn.date, true)} · <Money cents={txn.amount} />
        </SheetDescription>
      </div>
      <div className="rounded-lg bg-muted/60 px-3 py-2 text-xs">
        <div className="font-mono">{txn.raw}</div>
        <div className="mt-1 text-muted-foreground">
          {matchText(txn)}
          {txn.detail ? ` · ${txn.detail}` : ''}
          {txn.recovered ? ' · recovered from a second OCR pass' : ''}
          {txn.statementId ? (
            <>
              {' · '}
              <a className="underline" href={href('import', txn.statementId)}>
                statement p.{txn.page}
              </a>
            </>
          ) : null}
        </div>
      </div>
      {txn.needsReview && (
        <div className="flex items-start gap-2 rounded-md bg-[var(--warn)]/15 px-3 py-2 text-sm">
          <AlertCircle className="mt-0.5 size-4 shrink-0" />
          <span>
            {txn.merchantKey === 'grab'
              ? 'Grab charges don’t say whether it was a ride or food. Pick the category for this one.'
              : 'Not sure about this one — check the name and category.'}
          </span>
        </div>
      )}
      <Field label="Merchant">
        <Input value={merchant} onChange={(e) => setMerchant(e.target.value)} />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Category">
          <select className={selectCls} value={category} onChange={(e) => setCategory(e.target.value)}>
            {category === '' && <option value="">—</option>}
            {cats.list.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Type">
          <select className={selectCls} value={kind} onChange={(e) => setKind(e.target.value as Kind)}>
            {Object.entries(txnKindNames).map(([k, n]) => (
              <option key={k} value={k}>
                {n}
              </option>
            ))}
          </select>
        </Field>
      </div>
      {changedIdentity && txn.merchantKey && (
        <label className="flex items-start gap-2 rounded-md border px-3 py-2 text-sm">
          <input type="checkbox" className="mt-1" checked={similar} onChange={(e) => setSimilar(e.target.checked)} />
          <span>
            Apply to every “{txn.raw.split(' ').slice(0, 3).join(' ')}…” transaction, now and in future imports
            {ambiguousKeys.has(txn.merchantKey) && <span className="block text-xs text-muted-foreground">Usually best off for {txn.merchant}: its charges vary.</span>}
          </span>
        </label>
      )}
      {manual && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Date">
            <Input type="date" value={when} onChange={(e) => setWhen(e.target.value)} />
          </Field>
          <Field label="Amount (S$)">
            <Input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} />
          </Field>
        </div>
      )}
      {txn.kind !== 'payment' && txn.kind !== 'transfer' && (
        <div className="rounded-lg border px-3 py-2">
          <div className="flex items-center gap-3">
            <label htmlFor="my-cost" className="flex-1 text-sm">
              <span className="font-medium">Your cost</span>
              <span className="block text-xs text-muted-foreground">
                {shared
                  ? 'Set by the split below.'
                  : `What this really cost you, if not the ${money(txn.amount)} charged (e.g. part was paid back in cash).`}
              </span>
            </label>
            <div className="relative w-28">
              <span className="pointer-events-none absolute inset-y-0 left-2.5 flex items-center text-sm text-muted-foreground">S$</span>
              <Input
                id="my-cost"
                inputMode="decimal"
                className="tabular pl-8"
                value={shared ? toDollars(Math.abs(txn.myAmount)) || '0' : myCost}
                disabled={shared}
                onChange={(e) => {
                  setMyCost(e.target.value);
                  setResetCost(false);
                }}
                aria-invalid={costInvalid || undefined}
              />
            </div>
          </div>
          {!shared && (txn.myCost !== null || costChanged) && !resetCost && (
            <button
              type="button"
              className="mt-1 inline-flex items-center gap-1 text-xs text-primary hover:underline"
              onClick={() => {
                setResetCost(true);
                setMyCost(toDollars(Math.abs(txn.amount)));
              }}
            >
              <RotateCcw className="size-3" /> Back to the charged amount
            </button>
          )}
          {costInvalid && <p className="mt-1 text-xs text-[var(--bad)]">Between S$0 and the amount charged.</p>}
        </div>
      )}
      <SplitEditor txn={txn} />
      <Field label="Note">
        <textarea className={textareaCls} rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      <label className="flex items-center justify-between gap-3 text-sm">
        <span>
          One-off
          <span className="block text-xs text-muted-foreground">e.g. a yearly premium; can be spread over 12 months in the trend</span>
        </span>
        <Switch checked={oneOff} onCheckedChange={setOneOff} />
      </label>
      <label className="flex items-center justify-between gap-3 text-sm">
        <span>
          Leave out of insights
          <span className="block text-xs text-muted-foreground">Not your spending at all (e.g. bought entirely for someone). If you shared it, split it instead.</span>
        </span>
        <Switch checked={excluded} onCheckedChange={setExcluded} />
      </label>
      <ErrorNote error={err} />
      <div className="flex flex-wrap gap-2 pt-1">
        <Button onClick={() => void save()} disabled={patch.isPending || costInvalid}>
          Save
        </Button>
        {txn.needsReview && !changedIdentity && (
          <Button variant="outline" onClick={() => void save(true)} disabled={patch.isPending}>
            Looks right
          </Button>
        )}
        <Button variant="ghost" className="ml-auto text-destructive" onClick={() => void remove()}>
          <Trash2 className="size-4" /> Delete
        </Button>
      </div>
    </div>
  );
}

function matchText(t: Txn): string {
  switch (t.matchSource) {
    case 'rule':
      return 'Matched by your rule';
    case 'user':
      return 'Edited by you';
    case 'brand':
      return 'Known Singapore brand';
    case 'osm':
      return `OpenStreetMap match (${Math.round(t.confidence * 100)}%)`;
    case 'acra':
      return `ACRA registry match (${Math.round(t.confidence * 100)}%)`;
    case 'claude':
      return `Identified by Claude (${Math.round(t.confidence * 100)}%)`;
    case 'keyword':
      return 'Category guessed from the name';
    case 'special':
      return 'Statement line';
    default:
      return 'Unknown merchant';
  }
}
