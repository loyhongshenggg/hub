import { Button, cn, Input, Sheet, SheetContent, SheetDescription, SheetTitle } from '@hub/ui-kit';
import { Archive, Check, Flag, Loader2, Pause, PenLine, Play, Plus, Sparkles, Target, Trash2, Trophy, X } from 'lucide-react';
import { useId, useLayoutEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent } from 'react';
import { api, type Drafts, type GoalView } from './api';
import { BalanceLine, PeriodBars } from './charts';
import { blankGoal, GoalForm, goalFormError, type GoalDraft } from './goal-form';
import { areaNames, date, kindNames, money, monthYear, plural, toCents, today } from './format';
import { keys, queryClient, useCategories, useClaude, useGoal, useGoals, useReorderGoals, useSaveGoal } from './queries';
import { go, href, type Route } from './router';
import { Empty, ErrorNote, Field, Meter, Money, Section, StatusChip, textareaCls } from './ui';

export function GoalsPage({ route }: { route: Route }) {
  const { data: goals = [], isLoading } = useGoals();
  const [composing, setComposing] = useState(route.id === 'new');
  // The page stays mounted (app.tsx), so a later #/goals/new must open it too.
  const [seenId, setSeenId] = useState(route.id);
  if (route.id !== seenId) {
    setSeenId(route.id);
    if (route.id === 'new') setComposing(true);
  }
  const open = route.id && route.id !== 'new' ? Number(route.id) : null;

  const active = goals.filter((g) => g.status === 'active');
  const paused = goals.filter((g) => g.status === 'paused');
  const done = goals.filter((g) => g.status === 'achieved' || g.status === 'archived');

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-3">
        <h1 className="text-xl font-semibold tracking-tight">Goals</h1>
        <span className="text-sm text-muted-foreground">{plural(active.length, 'active goal')}</span>
        {!composing && (
          <Button className="ml-auto" onClick={() => setComposing(true)}>
            <Plus className="size-4" /> New goal
          </Button>
        )}
      </div>

      {composing && <Composer onClose={() => setComposing(false)} />}

      {isLoading ? null : active.length === 0 && !composing ? (
        <Empty icon={Target} title="No active goals">
          Describe what you're working towards — a safety net, a home, paying something off, spending less on something. Goals come and go; achieved ones stay
          in your history.
        </Empty>
      ) : (
        <SortableGoals goals={active} label="Active goals" />
      )}

      {paused.length > 0 && (
        <div>
          <h2 className="mb-2 text-sm font-medium text-muted-foreground">Paused</h2>
          <SortableGoals goals={paused} label="Paused goals" />
        </div>
      )}

      {done.length > 0 && <Timeline goals={done} />}

      <Sheet open={open !== null} onOpenChange={(o) => !o && go('goals')}>
        <SheetContent className="overflow-y-auto p-5 sm:max-w-xl">{open !== null && <GoalDetail id={open} />}</SheetContent>
      </Sheet>
    </div>
  );
}

/**
 * SortableGoals: a grid of goal cards you can reorder by dragging, or with
 * Alt + arrow keys on a focused card. The order is saved at once.
 *
 * Dragging stays steady because the drop slot comes from where the cards
 * will be (their layout boxes, which slide animations don't move), and the
 * card just swapped with is ignored until the pointer leaves it.
 */
function SortableGoals({ goals, label }: { goals: GoalView[]; label: string }) {
  const reorder = useReorderGoals();
  const [order, setOrder] = useState<number[] | null>(null); // while dragging or saving
  const [dragId, setDragId] = useState<number | null>(null);
  const grid = useRef<HTMLDivElement>(null);
  const startOrder = useRef<number[]>([]);
  const nodes = useRef(new Map<number, HTMLElement>());
  const from = useRef<Map<number, DOMRect> | null>(null); // positions before a reorder (FLIP)
  const settle = useRef<number | null>(null); // card just swapped with
  const refocus = useRef<number | null>(null);
  const hintId = useId();

  const serverIds = goals.map((g) => g.id);
  const sameSet = order !== null && order.length === serverIds.length && order.every((id) => serverIds.includes(id));
  if (order !== null && (!sameSet || (dragId === null && order.join() === serverIds.join()))) setOrder(null);
  const ids = sameSet && order ? order : serverIds;
  const byId = new Map(goals.map((g) => [g.id, g]));

  /** setIds reorders the cards, remembering where each one is on screen now. */
  const setIds = (next: number[]) => {
    from.current = new Map([...nodes.current].map(([id, el]) => [id, el.getBoundingClientRect()]));
    setOrder(next);
  };

  // Slide each moved card from where it was to its new place (FLIP),
  // continuing smoothly if it was already sliding. Off with reduced motion.
  useLayoutEffect(() => {
    const before = from.current;
    from.current = null;
    if (before && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      for (const [id, el] of nodes.current) {
        const was = before.get(id);
        if (!was) continue;
        el.getAnimations().forEach((a) => a.cancel());
        const now = el.getBoundingClientRect();
        const dx = was.left - now.left;
        const dy = was.top - now.top;
        if (dx || dy) {
          el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }], { duration: 200, easing: 'cubic-bezier(0.2, 0, 0, 1)' });
        }
      }
    }
    if (refocus.current !== null) {
      nodes.current.get(refocus.current)?.focus();
      refocus.current = null;
    }
  });

  const moveTo = (list: number[], id: number, to: number) => {
    const at = list.indexOf(id);
    if (at < 0 || to < 0 || to >= list.length || at === to) return list;
    const next = list.filter((x) => x !== id);
    next.splice(to, 0, id);
    return next;
  };
  const save = (next: number[]) => {
    setIds(next);
    reorder.mutate(next, { onError: () => setOrder(null) }); // back to the saved order
  };

  /** The card whose layout box (ignoring slide animations) holds the point. */
  const cardAt = (x: number, y: number): number | null => {
    const box = grid.current?.getBoundingClientRect();
    if (!box) return null;
    const px = x - box.left;
    const py = y - box.top;
    for (const [id, el] of nodes.current) {
      if (px >= el.offsetLeft && px < el.offsetLeft + el.offsetWidth && py >= el.offsetTop && py < el.offsetTop + el.offsetHeight) return id;
    }
    return null;
  };

  const onGridDragOver = (e: DragEvent) => {
    if (dragId === null) return; // a drag from elsewhere (another section, a file)
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const over = cardAt(e.clientX, e.clientY);
    if (over !== settle.current) settle.current = null; // left the card we swapped with
    if (over === null || over === dragId || over === settle.current) return;
    const next = moveTo(ids, dragId, ids.indexOf(over));
    if (next !== ids) {
      settle.current = over;
      setIds(next);
    }
  };

  const handlers = (id: number) => ({
    draggable: true,
    onDragStart: (e: DragEvent) => {
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', String(id));
      startOrder.current = ids;
      settle.current = null;
      setOrder(ids);
      setDragId(id);
    },
    onDragEnd: () => {
      setDragId(null);
      settle.current = null;
      if (ids.join() !== startOrder.current.join()) save(ids);
      else setOrder(null);
    },
    onKeyDown: (e: KeyboardEvent) => {
      if (!e.altKey) return;
      const step = e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : 0;
      if (!step) return;
      e.preventDefault();
      const next = moveTo(ids, id, ids.indexOf(id) + step);
      if (next !== ids) {
        refocus.current = id;
        save(next);
      }
    },
  });

  return (
    <>
      <div
        ref={grid}
        className="relative grid gap-3 sm:grid-cols-2 xl:grid-cols-3"
        role="list"
        aria-label={label}
        aria-describedby={hintId}
        onDragOver={onGridDragOver}
        onDrop={(e) => dragId !== null && e.preventDefault()}
      >
        {ids.map((id) => {
          const g = byId.get(id);
          return g ? (
            <div
              key={id}
              role="listitem"
              ref={(el) => {
                if (el) nodes.current.set(id, el);
                else nodes.current.delete(id);
              }}
              className="flex"
            >
              <GoalCard g={g} dragging={dragId === id} {...handlers(id)} />
            </div>
          ) : null;
        })}
      </div>
      <span id={hintId} className="sr-only">
        Drag a goal to reorder, or press Alt and an arrow key.
      </span>
      {reorder.error && <ErrorNote error={reorder.error} />}
    </>
  );
}

type CardDrag = {
  dragging?: boolean;
  draggable?: boolean;
  onDragStart?: (e: DragEvent) => void;
  onDragEnd?: () => void;
  onKeyDown?: (e: KeyboardEvent) => void;
};

function GoalCard({ g, dragging, ...drag }: { g: GoalView } & CardDrag) {
  const p = g.progress;
  const isLimit = g.kind === 'limit';
  const count = isLimit && g.metric === 'count';
  const fmt = (v: number) => (count ? String(v) : money(v, { whole: v >= 100000 || v % 100 === 0 }));
  return (
    <a
      href={href('goals', g.id)}
      {...drag}
      title={drag.draggable ? 'Drag to reorder' : undefined}
      className={cn(
        'flex w-full flex-col gap-3 rounded-xl border bg-card p-4 shadow-xs transition-[background-color,opacity] hover:bg-accent/50',
        dragging && 'opacity-40',
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="text-xs text-muted-foreground">
            {areaNames[g.area] ?? g.area} · {kindNames[g.kind]}
          </div>
          <div className="truncate font-semibold">{g.title}</div>
        </div>
        <StatusChip status={p.status} />
      </div>
      {g.why && <p className="line-clamp-2 text-sm text-muted-foreground italic">“{g.why}”</p>}
      <div className="mt-auto flex flex-col gap-1.5">
        <div className="flex items-baseline justify-between gap-2">
          <span className="tabular text-lg font-semibold">
            {g.kind === 'payoff' ? fmt(p.remaining) : fmt(p.current)}
            <span className="text-sm font-normal text-muted-foreground">
              {g.kind === 'payoff' ? ' left' : ` of ${fmt(g.target)}`}
              {isLimit || g.kind === 'habit' ? ` · ${p.periodLabel}` : ''}
            </span>
          </span>
          <span className="tabular text-sm text-muted-foreground">{Math.round(p.percent * 100)}%</span>
        </div>
        <Meter value={p.percent} tone={p.status === 'over' ? 'bad' : p.status === 'achieved' ? 'good' : 'seq'} label={g.title} />
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span className="line-clamp-1">{p.statusText}</span>
          {p.streak > 1 && <span className="shrink-0"><span role="img" aria-label="streak">🔥</span> {p.streak} in a row</span>}
        </div>
      </div>
    </a>
  );
}

function Timeline({ goals }: { goals: GoalView[] }) {
  const byYear = useMemo(() => {
    const m = new Map<string, GoalView[]>();
    for (const g of goals) {
      const y = (g.achievedAt ?? g.updatedAt ?? '').slice(0, 4) || '—';
      m.set(y, [...(m.get(y) ?? []), g]);
    }
    return [...m.entries()].sort((a, b) => b[0].localeCompare(a[0]));
  }, [goals]);
  return (
    <Section title={<span className="flex items-center gap-2"><Trophy className="size-4 text-[var(--warn)]" /> Your track record</span>}>
      <ol className="flex flex-col gap-4">
        {byYear.map(([year, gs]) => (
          <li key={year}>
            <div className="mb-1 text-xs font-semibold text-muted-foreground">{year}</div>
            <ul className="flex flex-col gap-1">
              {gs.map((g) => (
                <li key={g.id}>
                  <a href={href('goals', g.id)} className="flex items-center gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                    {g.status === 'achieved' ? <Check className="size-4 text-[var(--good)]" /> : <Archive className="size-4 text-muted-foreground" />}
                    <span className="flex-1 truncate">{g.title}</span>
                    <span className="text-xs text-muted-foreground">{g.status === 'achieved' ? `Achieved ${monthYear((g.achievedAt ?? '').slice(0, 10))}` : 'Archived'}</span>
                    {g.kind === 'target' && <Money cents={g.target} whole />}
                  </a>
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ol>
    </Section>
  );
}

// --- composer: paragraph → Claude drafts, or a blank form

function Composer({ onClose }: { onClose: () => void }) {
  const { data: claude } = useClaude();
  const aiReady = !!claude?.keySet && !!claude.goals;
  const [mode, setMode] = useState<'words' | 'form'>('words');
  const [text, setText] = useState('');
  const [share, setShare] = useState(true);
  const [drafts, setDrafts] = useState<Drafts | null>(null);
  const [forms, setForms] = useState<GoalDraft[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<unknown>(null);

  const draft = async () => {
    setBusy(true);
    setErr(null);
    try {
      const d = await api.draftGoals(text, share);
      if (d.drafts.length === 0) {
        setErr(new Error(d.questions[0] ?? 'No goals found in that paragraph.'));
        return;
      }
      setDrafts(d);
      setForms(d.drafts.map((g) => ({ ...g })));
    } catch (e) {
      setErr(e);
    } finally {
      setBusy(false);
    }
  };

  if (forms.length > 0) {
    return <DraftReview drafts={drafts} forms={forms} setForms={setForms} onDone={onClose} />;
  }

  return (
    <Section
      title={mode === 'words' ? 'Describe your goal' : 'New goal'}
      action={
        <Button variant="ghost" size="icon-sm" onClick={onClose} aria-label="Close">
          <X className="size-4" />
        </Button>
      }
    >
      {mode === 'words' ? (
        <div className="flex flex-col gap-3">
          <textarea
            className={cn(textareaCls, 'min-h-32 text-base')}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="e.g. I want a 6-month emergency fund of about S$18k by end of 2027 — I have S$6k now and can put aside S$700 a month. I also order too much food delivery; I'd like to keep it under S$200 a month."
          />
          {aiReady && (
            <label className="flex items-start gap-2 text-sm">
              <input type="checkbox" className="mt-1" checked={share} onChange={(e) => setShare(e.target.checked)} />
              <span>
                Share my average monthly spending per category so the targets are realistic
                <span className="block text-xs text-muted-foreground">
                  Sent to Claude: this paragraph{share ? ', category names and averages' : ' and category names'}. Never your transactions.
                </span>
              </span>
            </label>
          )}
          <ErrorNote error={err} />
          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => void draft()} disabled={busy || text.trim().length < 10}>
              {busy ? <Loader2 className="size-4 animate-spin" /> : <Sparkles className="size-4" />}
              {busy ? 'Reading…' : 'Turn into goals'}
            </Button>
            <Button variant="ghost" onClick={() => setMode('form')}>
              <PenLine className="size-4" /> Fill in a form instead
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {aiReady ? (
              'Read by Claude; if it is unavailable, read on this computer instead. You check every goal before it is saved.'
            ) : (
              <>
                Read on this computer: amounts (S$18k, $700 a month), dates (by July 2029, end of 2027, in 2 years) and intent (save, invest, pay off, keep
                under). You check every goal before it is saved. Optional: an Anthropic key in{' '}
                <a className="text-primary underline-offset-4 hover:underline" href={href('settings')}>
                  Settings
                </a>{' '}
                lets Claude read longer, looser paragraphs.
              </>
            )}
          </p>
        </div>
      ) : (
        <SingleForm onDone={onClose} />
      )}
    </Section>
  );
}

function SingleForm({ onDone }: { onDone: () => void }) {
  const [g, setG] = useState<GoalDraft>({ ...blankGoal(), startDate: today() });
  const save = useSaveGoal();
  return (
    <div className="flex flex-col gap-4">
      <GoalForm value={g} onChange={setG} />
      <FormProblem draft={g} />
      <ErrorNote error={save.error} />
      <div className="flex gap-2">
        <Button disabled={save.isPending || !!goalFormError(g)} onClick={() => save.mutate(g, { onSuccess: onDone })}>
          Save goal
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

function DraftReview({ drafts, forms, setForms, onDone }: { drafts: Drafts | null; forms: GoalDraft[]; setForms: (f: GoalDraft[]) => void; onDone: () => void }) {
  const save = useSaveGoal();
  const [saved, setSaved] = useState<Set<number>>(new Set());
  // Skipped drafts are hidden, not removed, so every form keeps its index.
  const [skipped, setSkipped] = useState<Set<number>>(new Set());
  const [err, setErr] = useState<unknown>(null);
  const finishIfDone = (s: Set<number>, k: Set<number>) => {
    if (s.size + k.size >= forms.length) onDone();
  };
  const saveOne = async (i: number) => {
    setErr(null);
    try {
      await save.mutateAsync(forms[i]);
      const next = new Set(saved).add(i);
      setSaved(next);
      finishIfDone(next, skipped);
    } catch (e) {
      setErr(e);
    }
  };
  return (
    <Section
      title={
        <span className="flex items-center gap-2">
          <Sparkles className="size-4 text-[var(--seq)]" /> {plural(forms.length, 'goal')} from your words — check and save
          {drafts?.engine === 'claude' && <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-normal text-muted-foreground">via Claude</span>}
        </span>
      }
      action={
        <Button variant="ghost" size="icon-sm" onClick={onDone} aria-label="Discard">
          <X className="size-4" />
        </Button>
      }
    >
      {drafts && (drafts.assumptions.length > 0 || drafts.questions.length > 0) && (
        <div className="mb-4 grid gap-3 rounded-lg bg-muted/60 p-3 text-sm sm:grid-cols-2">
          {drafts.assumptions.length > 0 && (
            <div>
              <div className="mb-1 font-medium">Assumed</div>
              <ul className="list-disc pl-4 text-muted-foreground">
                {drafts.assumptions.map((a) => (
                  <li key={a}>{a}</li>
                ))}
              </ul>
            </div>
          )}
          {drafts.questions.length > 0 && (
            <div>
              <div className="mb-1 font-medium">Worth deciding</div>
              <ul className="list-disc pl-4 text-muted-foreground">
                {drafts.questions.map((q) => (
                  <li key={q}>{q}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
      <ErrorNote error={err} />
      <div className="flex flex-col gap-6">
        {forms.map((f, i) =>
          skipped.has(i) ? null : saved.has(i) ? (
            <div key={i} className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm">
              <Check className="size-4 text-[var(--good)]" /> Saved “{f.title}”
            </div>
          ) : (
            <div key={i} className="rounded-xl border p-4">
              {drafts?.reasoning[i] && <p className="mb-3 text-xs text-muted-foreground">{drafts.reasoning[i]}</p>}
              <GoalForm value={f} onChange={(g) => setForms(forms.map((x, j) => (j === i ? g : x)))} />
              <FormProblem draft={f} />
              <div className="mt-4 flex gap-2">
                <Button onClick={() => void saveOne(i)} disabled={save.isPending || !!goalFormError(f)}>
                  Save this goal
                </Button>
                <Button
                  variant="ghost"
                  onClick={() => {
                    const next = new Set(skipped).add(i);
                    setSkipped(next);
                    finishIfDone(saved, next);
                  }}
                >
                  Skip
                </Button>
              </div>
            </div>
          ),
        )}
      </div>
    </Section>
  );
}

// --- detail sheet

function GoalDetail({ id }: { id: number }) {
  const { data: g, error } = useGoal(id);
  const save = useSaveGoal();
  const cats = useCategories();
  const [editing, setEditing] = useState<GoalDraft | null>(null);
  if (error) return <ErrorNote error={error} />;
  if (!g) return <Loader2 className="m-8 size-5 animate-spin text-muted-foreground" />;
  const p = g.progress;
  const setStatus = (status: GoalView['status']) => save.mutate({ ...g, status });

  if (editing) {
    return (
      <div className="flex flex-col gap-4">
        <SheetTitle>Edit goal</SheetTitle>
        <GoalForm value={editing} onChange={setEditing} />
        <FormProblem draft={editing} />
        <ErrorNote error={save.error} />
        <div className="flex gap-2">
          <Button onClick={() => save.mutate({ ...g, ...editing }, { onSuccess: () => setEditing(null) })} disabled={save.isPending || !!goalFormError(editing)}>
            Save
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              save.reset();
              setEditing(null);
            }}
          >
            Cancel
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <div className="pr-8">
        <div className="text-xs text-muted-foreground">
          {areaNames[g.area] ?? g.area} · {kindNames[g.kind]}
          {g.source === 'claude' ? ' · drafted with Claude' : ''}
        </div>
        <SheetTitle className="text-xl">{g.title}</SheetTitle>
        <SheetDescription className="mt-1 flex flex-wrap items-center gap-2">
          <StatusChip status={p.status} />
          <span>{p.statusText}</span>
        </SheetDescription>
      </div>
      {g.why && <blockquote className="border-l-2 pl-3 text-sm italic text-muted-foreground">“{g.why}”</blockquote>}

      <Numbers g={g} />

      {(g.kind === 'target' || g.kind === 'payoff') && p.history.length > 1 && <BalanceLine points={p.history} target={g.target} payoff={g.kind === 'payoff'} />}
      {(g.kind === 'limit' || g.kind === 'habit') && p.history.length > 0 && (
        <PeriodBars points={p.history} target={g.target} count={g.metric === 'count'} mode={g.kind} />
      )}

      {g.kind === 'limit' && (
        <div className="text-sm text-muted-foreground">
          Counts{' '}
          {[
            ...(g.filter.categories ?? []).map((c) => cats.get(c).name),
            ...(g.filter.merchants ?? []),
            ...(g.filter.keywords ?? []).map((k) => `“${k}”`),
          ].join(', ') || 'all spending'}
          .{' '}
          <a className="text-primary underline-offset-4 hover:underline" href={href('transactions', undefined, { goal: g.id, month: g.period === 'month' ? p.periodStart?.slice(0, 7) : undefined })}>
            See transactions
          </a>
        </div>
      )}

      {g.kind !== 'limit' && g.status === 'active' && <CheckIn g={g} />}

      {p.milestones.length > 0 && (
        <div>
          <h3 className="mb-2 text-sm font-medium">Milestones</h3>
          <ul className="flex flex-col gap-1.5">
            {p.milestones.map((m, i) => (
              <li key={i} className="flex items-center gap-2 text-sm">
                <button
                  type="button"
                  disabled={!!m.amount}
                  onClick={() => save.mutate({ ...g, milestones: g.milestones.map((x, j) => (j === i ? { ...x, doneAt: x.doneAt ? undefined : today() } : x)) })}
                  className={cn('flex size-5 items-center justify-center rounded-full border', m.reached && 'border-[var(--good)] bg-[var(--good)] text-white')}
                  aria-label={m.reached ? 'Reached' : 'Not yet'}
                >
                  {m.reached ? <Check className="size-3" /> : <Flag className="size-3 text-muted-foreground" />}
                </button>
                <span className={cn('flex-1', m.reached && 'text-muted-foreground line-through')}>{m.label}</span>
                {m.amount ? <Money cents={m.amount} whole /> : null}
                {m.date ? <span className="text-xs text-muted-foreground">{date(m.date, true)}</span> : null}
              </li>
            ))}
          </ul>
        </div>
      )}

      {g.entries && g.entries.length > 0 && <Entries g={g} />}

      <div className="flex flex-wrap gap-2 border-t pt-4">
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            save.reset();
            setEditing({ ...g });
          }}
        >
          <PenLine className="size-4" /> Edit
        </Button>
        {g.status === 'active' && (
          <Button variant="outline" size="sm" onClick={() => setStatus('achieved')}>
            <Trophy className="size-4" /> Mark achieved
          </Button>
        )}
        {g.status === 'active' && (
          <Button variant="ghost" size="sm" onClick={() => setStatus('paused')}>
            <Pause className="size-4" /> Pause
          </Button>
        )}
        {g.status !== 'active' && (
          <Button variant="outline" size="sm" onClick={() => setStatus('active')}>
            <Play className="size-4" /> Make active
          </Button>
        )}
        {g.status !== 'archived' && (
          <Button variant="ghost" size="sm" onClick={() => setStatus('archived')}>
            <Archive className="size-4" /> Archive
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto text-destructive"
          onClick={async () => {
            if (window.confirm(`Delete “${g.title}” and its history? Archiving keeps it in your track record.`)) {
              await api.deleteGoal(g.id);
              void queryClient.invalidateQueries({ queryKey: keys.goals });
              go('goals');
            }
          }}
        >
          <Trash2 className="size-4" /> Delete
        </Button>
      </div>
      {g.prompt && (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">Your original words</summary>
          <p className="mt-1 whitespace-pre-wrap">{g.prompt}</p>
        </details>
      )}
    </div>
  );
}

function Numbers({ g }: { g: GoalView }) {
  const p = g.progress;
  const count = g.metric === 'count';
  const f = (v: number) => (count ? String(v) : money(v, { whole: v >= 100000 || v % 100 === 0 }));
  const cells: [string, string][] = [];
  switch (g.kind) {
    case 'target':
      cells.push(['Saved', f(p.current)], ['Target', f(g.target)], ['To go', f(p.remaining)]);
      if (p.neededPerMonth) cells.push(['Needed / month', f(p.neededPerMonth)]);
      if (p.projectedDate) cells.push(['At this pace', monthYear(p.projectedDate)]);
      if (g.targetDate) cells.push(['Target date', date(g.targetDate, true)]);
      break;
    case 'payoff':
      cells.push(['Left', f(p.remaining)], ['Started at', f(g.startAmount)], ['Paid', f(g.startAmount - p.current)]);
      if (p.neededPerMonth) cells.push(['Needed / month', f(p.neededPerMonth)]);
      if (p.projectedDate) cells.push(['Paid off by', monthYear(p.projectedDate)]);
      break;
    case 'limit':
      cells.push([p.periodLabel ?? 'This period', f(p.current)], ['Limit', f(g.target)]);
      if (p.projected && p.projected !== p.current) cells.push(['On pace for', f(p.projected)]);
      cells.push(['Streak', plural(p.streak, g.period)]);
      break;
    case 'habit':
      cells.push([p.periodLabel ?? 'This period', f(p.current)], ['Goal', f(g.target)], ['Streak', plural(p.streak, g.period)]);
      break;
  }
  return (
    <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
      {cells.map(([k, v]) => (
        <div key={k} className="rounded-lg bg-muted/60 px-3 py-2">
          <dt className="text-xs text-muted-foreground">{k}</dt>
          <dd className="tabular font-semibold">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

function CheckIn({ g }: { g: GoalView }) {
  const [amount, setAmount] = useState('');
  const [when, setWhen] = useState(today());
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<unknown>(null);
  const save = useSaveGoal();
  const label = g.kind === 'target' ? 'How much is in it now?' : g.kind === 'payoff' ? 'Balance left now' : 'Amount put in';
  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api.addEntry(g.id, { date: when, amount: toCents(amount), note });
      setAmount('');
      setNote('');
      save.reset();
      void queryClient.invalidateQueries({ queryKey: keys.goal(g.id) });
      void queryClient.invalidateQueries({ queryKey: keys.goals });
    } catch (e) {
      setErr(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="rounded-xl border p-3">
      <h3 className="mb-2 text-sm font-medium">Check in</h3>
      <div className="grid grid-cols-[1fr_9rem] gap-2 max-[480px]:grid-cols-1">
        <Field label={label}>
          <Input inputMode="decimal" className="tabular" placeholder="S$" value={amount} onChange={(e) => setAmount(e.target.value)} />
        </Field>
        <Field label="Date">
          <Input type="date" value={when} onChange={(e) => setWhen(e.target.value)} />
        </Field>
      </div>
      {amount.trim() !== '' && !(toCents(amount) >= 0) && <p className="mt-1 text-xs text-destructive">Type an amount, e.g. 37500, 37,500.50 or 37.5k</p>}
      <Input className="mt-2" placeholder="Note (optional)" value={note} onChange={(e) => setNote(e.target.value)} />
      <ErrorNote error={err} />
      <Button className="mt-2" size="sm" onClick={() => void submit()} disabled={busy || !amount || !(toCents(amount) >= 0)}>
        Add check-in
      </Button>
    </div>
  );
}

function Entries({ g }: { g: GoalView }) {
  const [list, setList] = useState(g.entries ?? []);
  return (
    <div>
      <h3 className="mb-2 text-sm font-medium">History</h3>
      <ul className="divide-y rounded-lg border text-sm">
        {[...list].reverse().map((e) => (
          <li key={e.id} className="flex items-center gap-3 px-3 py-1.5">
            <span className="w-24 text-xs text-muted-foreground">{date(e.date, true)}</span>
            <span className="flex-1 truncate text-muted-foreground">{e.note}</span>
            <Money cents={e.amount} />
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label="Delete check-in"
              onClick={async () => {
                const v = await api.deleteEntry(g.id, e.id);
                setList(v.entries ?? []);
                void queryClient.invalidateQueries({ queryKey: keys.goal(g.id) });
                void queryClient.invalidateQueries({ queryKey: keys.goals });
              }}
            >
              <X className="size-3" />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** FormProblem says why Save is off (see goalFormError). */
function FormProblem({ draft }: { draft: GoalDraft }) {
  const problem = goalFormError(draft);
  return problem ? <p className="text-sm text-muted-foreground">{problem}</p> : null;
}
