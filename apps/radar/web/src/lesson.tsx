// Focus mode for a chapter: the step list, the resource, its focus note and
// one primary action (Done). Dashboard chrome is hidden.
import { Button, cn } from '@hub/ui-kit';
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CircleDashed,
  PartyPopper,
  RotateCcw,
  Sparkles,
  ThumbsDown,
  ThumbsUp,
  X,
} from 'lucide-react';
import { ViewTransition, useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { ChapterView, Rating, StepView } from './api';
import {
  afterDone,
  nextOf,
  prefetchReader,
  rowsOf,
  useChapter,
  useCheckpoint,
  useDashboard,
  useDismiss,
  useDone,
  useSkipToPractice,
  useUndo,
} from './queries';
import { navigate } from './router';
import {
  Difficulty,
  Kbd,
  ProgressBar,
  Segmented,
  Skeleton,
  TypeIcon,
  minutes,
  typeLabel,
  useDelayed,
} from './ui';
import { Viewer } from './viewer';

const ratings = [
  { value: 'easy' as const, label: 'Too easy' },
  { value: 'right' as const, label: 'Just right' },
  { value: 'hard' as const, label: 'Too hard' },
];

export function Lesson({
  track,
  chapter,
  step,
}: {
  track: string;
  chapter: string;
  step?: string;
}) {
  const q = useChapter(track, chapter);
  const slow = useDelayed(q.isPending);
  if (q.isError)
    return (
      <div className="p-8 text-sm text-muted-foreground">
        {q.error.message}{' '}
        <Button variant="link" onClick={() => navigate({ name: 'home' })}>
          Back to dashboard
        </Button>
      </div>
    );
  if (!q.data)
    return slow ? (
      <div className="mx-auto max-w-5xl space-y-4 p-6">
        <Skeleton className="h-6 w-1/2" />
        <Skeleton className="h-64" />
      </div>
    ) : null;
  return <LessonBody view={q.data} step={step} />;
}

function LessonBody({ view, step }: { view: ChapterView; step?: string }) {
  const qc = useQueryClient();
  const rows = rowsOf(view);
  const current =
    rows.find((r) => r.ref === step) ?? view.next ?? rows[0] ?? view.checkpoint;
  const index = rows.findIndex((r) => r.ref === current.ref);
  const go = (ref: string | null | undefined) =>
    ref &&
    navigate(
      { name: 'lesson', track: view.track, chapter: view.chapter, step: ref },
      { replace: true },
    );

  // Prefetch the reader for the step after this one.
  useEffect(() => {
    prefetchReader(qc, rows[index + 1]?.ref);
  }, [qc, rows, index]);

  // Keyboard: j/k next/previous, o open outside, Esc back (d lives in ActionBar).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTyping(e.target)) return;
      if (e.key === 'j') go(rows[Math.min(rows.length - 1, index + 1)]?.ref);
      else if (e.key === 'k') go(rows[Math.max(0, index - 1)]?.ref);
      else if (e.key === 'o' && current.url)
        window.open(current.url, '_blank', 'noopener,noreferrer');
      else if (e.key === 'Escape') navigate({ name: 'home' });
      else return;
      e.preventDefault();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  return (
    <div className="@container min-h-dvh">
      <header className="sticky top-0 z-20 border-b bg-background/85 backdrop-blur supports-[backdrop-filter]:bg-background/70">
        <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-2.5">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Back to dashboard"
            title="Back (Esc)"
            onClick={() => navigate({ name: 'home' })}
          >
            <ArrowLeft />
          </Button>
          <div className="min-w-0 flex-1">
            <button
              className="block truncate text-xs text-muted-foreground hover:text-foreground"
              onClick={() => navigate({ name: 'track', track: view.track })}
            >
              {view.trackTitle} · Chapter {view.index + 1} of {view.count}
            </button>
            <ViewTransition name={`chapter-${view.track}-${view.chapter}`}>
              <h1 className="truncate text-sm font-semibold @2xl:text-base">
                {view.title}
              </h1>
            </ViewTransition>
          </div>
          <div className="hidden w-40 shrink-0 items-center gap-2 @2xl:flex">
            <ProgressBar
              value={view.stepsDone}
              max={view.stepsTotal}
              className="flex-1"
            />
            <span className="text-xs text-muted-foreground tabular-nums">
              {view.stepsDone}/{view.stepsTotal}
            </span>
          </div>
        </div>
      </header>

      <div className="mx-auto grid max-w-6xl gap-6 px-4 py-6 @4xl:grid-cols-[16.5rem_minmax(0,1fr)]">
        <StepRail view={view} rows={rows} current={current.ref} onSelect={go} />
        <main className="@container min-w-0">
          {view.offerSkip &&
            current.kind !== 'practice' &&
            current.kind !== 'checkpoint' && (
              <SkipOffer view={view} onSkipped={(v) => go(nextOf(v)?.ref)} />
            )}
          <ViewTransition
            key={current.ref}
            enter="vt-screen"
            exit="vt-screen"
            default="none"
          >
            <div>
              {current.kind === 'checkpoint' ? (
                <Checkpoint view={view} />
              ) : (
                <StepPanel view={view} row={current} onNext={go} />
              )}
            </div>
          </ViewTransition>
        </main>
      </div>
    </div>
  );
}

function isTyping(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null;
  return (
    !!el &&
    (el.isContentEditable ||
      ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))
  );
}

function StepRail({
  view,
  rows,
  current,
  onSelect,
}: {
  view: ChapterView;
  rows: StepView[];
  current: string;
  onSelect: (ref: string) => void;
}) {
  let n = 0;
  return (
    <nav aria-label="Steps" className="@4xl:sticky @4xl:top-20 @4xl:self-start">
      <div className="mb-3 flex items-center gap-3 text-xs text-muted-foreground">
        <Difficulty level={view.difficulty} />
        <span>{minutes(view.minutes)}</span>
        <span className="@4xl:hidden">
          · {view.stepsDone}/{view.stepsTotal} done
        </span>
      </div>
      <ol className="flex gap-1 overflow-x-auto pb-1 @4xl:flex-col @4xl:overflow-visible">
        {rows.map((r) => {
          const num = r.kind === 'step' ? ++n : null;
          const on = r.ref === current;
          const done =
            r.status === 'done' || (r.kind === 'checkpoint' && view.completed);
          return (
            <li key={r.ref} className="shrink-0 @4xl:shrink">
              <button
                onClick={() => onSelect(r.ref)}
                aria-current={on ? 'step' : undefined}
                className={cn(
                  'flex w-full items-start gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-colors duration-(--dur-fast) hover:bg-accent',
                  on && 'bg-accent font-medium',
                  r.status === 'skipped' && 'opacity-60',
                )}
              >
                <span
                  className={cn(
                    'mt-0.5 grid size-5 shrink-0 place-items-center rounded-full border text-[11px] tabular-nums',
                    done && 'border-success bg-success text-white',
                    !done && r.kind !== 'step' && 'border-dashed',
                  )}
                >
                  {done ? (
                    <Check className="size-3" strokeWidth={3} />
                  ) : num ? (
                    num
                  ) : (
                    <TypeIcon type={r.type} kind={r.kind} className="size-3" />
                  )}
                </span>
                <span className="hidden min-w-0 @4xl:block">
                  <span className="line-clamp-2">{r.title}</span>
                  {(r.kind === 'corrective' || r.kind === 'fresh') && (
                    <span className="mt-0.5 flex items-center gap-1 text-xs font-normal text-muted-foreground">
                      <Sparkles className="size-3" />
                      {r.kind === 'fresh'
                        ? 'Fresh · optional'
                        : 'Added for you'}
                    </span>
                  )}
                </span>
                <span className="max-w-40 truncate @4xl:hidden">{r.title}</span>
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

function SkipOffer({
  view,
  onSkipped,
}: {
  view: ChapterView;
  onSkipped: (v: ChapterView) => void;
}) {
  const skip = useSkipToPractice();
  const [hidden, setHidden] = useState(false);
  if (hidden) return null;
  return (
    <div className="mb-5 flex flex-col gap-3 rounded-xl border border-primary/30 bg-primary/5 p-4 text-sm animate-in fade-in slide-in-from-top-1 duration-(--dur-base) @lg:flex-row @lg:items-center">
      <p className="flex-1">
        That felt easy. Skip the rest of the reading and go straight to{' '}
        {view.practice ? 'the practice' : 'the checkpoint'}?
      </p>
      <div className="flex gap-2">
        <Button variant="ghost" size="sm" onClick={() => setHidden(true)}>
          Keep reading
        </Button>
        <Button
          size="sm"
          disabled={skip.isPending}
          onClick={() =>
            skip.mutate(
              { track: view.track, chapter: view.chapter },
              { onSuccess: (v) => v && onSkipped(v) },
            )
          }
        >
          Skip ahead <ArrowRight />
        </Button>
      </div>
    </div>
  );
}

function StepPanel({
  view,
  row,
  onNext,
}: {
  view: ChapterView;
  row: StepView;
  onNext: (ref: string | null) => void;
}) {
  const [checked, setChecked] = useChecklist(row.ref, row.steps?.length ?? 0);
  return (
    <div className="space-y-5 pb-28">
      <div className="space-y-2">
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs font-medium text-muted-foreground">
          <TypeIcon type={row.type} kind={row.kind} className="size-3.5" />
          {row.kind === 'practice'
            ? 'Practice'
            : (typeLabel[row.type] ?? 'Read')}
          {row.minutes > 0 && <span>· {minutes(row.minutes)}</span>}
          {row.by && <span>· {row.by}</span>}
        </p>
        <h2 className="text-xl font-semibold tracking-tight text-balance @2xl:text-2xl">
          {row.title}
        </h2>
        {row.reason && (
          <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <Sparkles className="size-3.5" /> {row.reason}
          </p>
        )}
      </div>

      {row.focus && (
        <aside className="rounded-xl border-l-4 border-primary bg-primary/5 px-4 py-3 text-sm leading-relaxed">
          <span className="font-semibold">Focus · </span>
          {row.focus}
        </aside>
      )}

      {row.steps && row.steps.length > 0 && (
        <ol className="space-y-2 rounded-xl border bg-card p-4">
          {row.steps.map((s, i) => (
            <li key={i}>
              <label className="flex cursor-pointer items-start gap-3 text-sm leading-relaxed">
                <input
                  type="checkbox"
                  checked={checked[i] ?? false}
                  onChange={(e) => setChecked(i, e.target.checked)}
                  className="mt-1 size-4 shrink-0 accent-(--primary)"
                />
                <span
                  className={cn(
                    checked[i] && 'text-muted-foreground line-through',
                  )}
                >
                  {s}
                </span>
              </label>
            </li>
          ))}
        </ol>
      )}

      {row.url && <Viewer refId={row.ref} compact />}

      <ActionBar view={view} row={row} onNext={onNext} />
    </div>
  );
}

/** Walkthrough ticks are a per-device convenience, kept in localStorage. */
function useChecklist(
  ref: string,
  n: number,
): [boolean[], (i: number, on: boolean) => void] {
  const key = `radar:check:${ref}`;
  const [state, setState] = useState<{ key: string; v: boolean[] }>(() => ({
    key,
    v: load(key, n),
  }));
  const v = state.key === key ? state.v : load(key, n);
  return [
    v,
    (i, on) => {
      const next = [...v];
      next[i] = on;
      setState({ key, v: next });
      try {
        localStorage.setItem(key, JSON.stringify(next));
      } catch {
        // storage unavailable: keep it in memory
      }
    },
  ];
}

function load(key: string, n: number): boolean[] {
  try {
    const v = JSON.parse(localStorage.getItem(key) ?? '[]') as boolean[];
    return Array.from({ length: n }, (_, i) => !!v[i]);
  } catch {
    return Array(n).fill(false) as boolean[];
  }
}

function ActionBar({
  view,
  row,
  onNext,
}: {
  view: ChapterView;
  row: StepView;
  onNext: (ref: string | null) => void;
}) {
  const done = useDone();
  const undo = useUndo();
  const dismiss = useDismiss();
  const [rating, setRating] = useState<Rating>('');
  const [worth, setWorth] = useState<boolean | null>(null);
  const finished = row.status === 'done';

  const finish = () => {
    const next = afterDone(view, row.ref);
    done.mutate({
      ref: row.ref,
      rating,
      worth,
      track: view.track,
      chapter: view.chapter,
    });
    onNext(next);
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (
        e.key !== 'd' ||
        e.metaKey ||
        e.ctrlKey ||
        e.altKey ||
        isTyping(e.target)
      )
        return;
      e.preventDefault();
      if (!finished) finish();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  return (
    <div className="sticky bottom-3 z-10 mt-8">
      <div className="flex flex-col gap-3 rounded-2xl border bg-background/90 p-3 shadow-lg backdrop-blur supports-[backdrop-filter]:bg-background/75 @xl:flex-row @xl:items-center">
        {finished ? (
          <>
            <p className="flex flex-1 items-center gap-2 text-sm">
              <span className="grid size-6 place-items-center rounded-full bg-success text-white">
                <Check className="size-3.5" strokeWidth={3} />
              </span>
              Done
              {row.rating
                ? ` · ${ratings.find((r) => r.value === row.rating)?.label.toLowerCase()}`
                : ''}
            </p>
            <div className="flex gap-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => undo.mutate(row.ref)}
              >
                <RotateCcw /> Undo
              </Button>
              <Button
                size="sm"
                onClick={() => onNext(afterDone(view, row.ref))}
              >
                Next <ArrowRight />
              </Button>
            </div>
          </>
        ) : (
          <>
            <div className="flex flex-1 flex-wrap items-center gap-2">
              <Segmented
                label="How did it feel?"
                options={ratings}
                value={rating}
                onChange={(v) => setRating(v as Rating)}
                clearable
                size="sm"
              />
              <div
                className="inline-flex rounded-lg bg-muted p-0.5"
                role="group"
                aria-label="Worth it?"
              >
                {[true, false].map((w) => (
                  <button
                    key={String(w)}
                    type="button"
                    aria-pressed={worth === w}
                    title={w ? 'Worth it' : 'Not worth it'}
                    onClick={() => setWorth(worth === w ? null : w)}
                    className={cn(
                      'rounded-md px-2 py-1 text-muted-foreground transition-colors duration-(--dur-fast) hover:text-foreground',
                      worth === w && 'bg-background text-foreground shadow-xs',
                    )}
                  >
                    {w ? (
                      <ThumbsUp className="size-3.5" />
                    ) : (
                      <ThumbsDown className="size-3.5" />
                    )}
                  </button>
                ))}
              </div>
            </div>
            <div className="flex gap-2">
              {row.optional && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    dismiss.mutate(row.ref);
                    onNext(afterDone(view, row.ref));
                  }}
                >
                  <X /> Not for me
                </Button>
              )}
              <Button onClick={finish} className="min-w-28">
                <Check /> Done <Kbd>d</Kbd>
              </Button>
            </div>
          </>
        )}
      </div>
      {done.isError && (
        <p className="mt-2 text-center text-xs text-destructive">
          Couldn't save that: {done.error.message}
        </p>
      )}
    </div>
  );
}

function Checkpoint({ view }: { view: ChapterView }) {
  const cp = useCheckpoint();
  const [met, setMet] = useState<boolean[]>(() =>
    view.outcomes.map((_, i) => view.met[i] ?? false),
  );
  const [rating, setRating] = useState<Rating>('');
  const [worth, setWorth] = useState<boolean | null>(null);
  const res = cp.data?.result;

  if (view.completed)
    return <Celebration view={view} fresh={!!res?.completed} />;

  const submit = (force: boolean) =>
    cp.mutate({
      track: view.track,
      chapter: view.chapter,
      met,
      rating,
      worth,
      force,
    });
  const corrective = res?.corrective
    ? view.steps.find((s) => s.ref === res.corrective)
    : undefined;

  return (
    <div className="mx-auto max-w-2xl space-y-6 pb-16">
      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">Checkpoint</p>
        <h2 className="text-2xl font-semibold tracking-tight">
          What can you do now?
        </h2>
        <p className="text-sm text-muted-foreground">
          Tick what you could explain or do without the material open. Be honest
          — anything left unticked gets one more angle before you move on.
        </p>
      </div>
      <ul className="space-y-2">
        {view.outcomes.map((o, i) => (
          <li key={i}>
            <button
              type="button"
              role="checkbox"
              aria-checked={met[i]}
              onClick={() => setMet((m) => m.map((v, j) => (j === i ? !v : v)))}
              className={cn(
                'flex w-full items-start gap-3 rounded-xl border p-4 text-left text-sm leading-relaxed transition-colors duration-(--dur-fast) hover:bg-accent',
                met[i] && 'border-success/50 bg-success/5',
              )}
            >
              <span
                className={cn(
                  'mt-0.5 grid size-5 shrink-0 place-items-center rounded-md border transition-colors',
                  met[i] && 'border-success bg-success text-white',
                )}
              >
                {met[i] && (
                  <Check
                    className="size-3.5 animate-in zoom-in-50 duration-(--dur-fast)"
                    strokeWidth={3}
                  />
                )}
              </span>
              <span>I can {o.charAt(0).toLowerCase() + o.slice(1)}</span>
            </button>
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap items-center gap-3 text-sm">
        <span className="text-muted-foreground">The chapter felt</span>
        <Segmented
          label="How did the chapter feel?"
          options={ratings}
          value={rating}
          onChange={(v) => setRating(v as Rating)}
          clearable
          size="sm"
        />
        <span className="text-muted-foreground">Worth it?</span>
        <Segmented
          label="Worth it?"
          options={[
            { value: 'yes', label: 'Yes' },
            { value: 'no', label: 'No' },
          ]}
          value={worth === null ? '' : worth ? 'yes' : 'no'}
          onChange={(v) => setWorth(v === '' ? null : v === 'yes')}
          clearable
          size="sm"
        />
      </div>

      {res && !res.completed ? (
        <div className="space-y-3 rounded-xl border border-warning/50 bg-warning/10 p-4 text-sm animate-in fade-in slide-in-from-bottom-1 duration-(--dur-base)">
          <p>
            Not quite yet.{' '}
            {corrective
              ? `Radar added “${corrective.title}” to this chapter — another angle on it.`
              : 'Revisit the steps above, then check again.'}
          </p>
          <div className="flex flex-wrap gap-2">
            {corrective && (
              <Button
                size="sm"
                onClick={() =>
                  navigate(
                    {
                      name: 'lesson',
                      track: view.track,
                      chapter: view.chapter,
                      step: corrective.ref,
                    },
                    { replace: true },
                  )
                }
              >
                Go to it <ArrowRight />
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={() => submit(true)}>
              Continue anyway
            </Button>
          </div>
        </div>
      ) : (
        <Button size="lg" disabled={cp.isPending} onClick={() => submit(false)}>
          <Check /> Complete chapter
        </Button>
      )}
      {cp.isError && (
        <p className="text-xs text-destructive">{cp.error.message}</p>
      )}
    </div>
  );
}

/** Chapter done: a calm moment, then the next thing. */
function Celebration({ view, fresh }: { view: ChapterView; fresh: boolean }) {
  const dash = useDashboard();
  const next = dash.data?.continue;
  return (
    <div className="mx-auto max-w-xl space-y-6 py-6 text-center">
      <div
        className={cn(
          'mx-auto grid size-16 place-items-center rounded-full bg-success text-white',
          fresh && 'animate-in zoom-in-50 spin-in-12 duration-(--dur-slow)',
        )}
      >
        {fresh ? (
          <PartyPopper className="size-7" />
        ) : (
          <Check className="size-7" strokeWidth={3} />
        )}
      </div>
      <div className="space-y-1">
        <h2 className="text-2xl font-semibold tracking-tight">
          Chapter complete
        </h2>
        <p className="text-sm text-muted-foreground">{view.title}</p>
      </div>
      <ul className="space-y-1.5 text-left text-sm">
        {view.outcomes.map((o, i) => (
          <li key={i} className="flex gap-2">
            {view.met[i] ? (
              <Check className="mt-0.5 size-4 shrink-0 text-success" />
            ) : (
              <CircleDashed className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            )}
            <span>{o}</span>
          </li>
        ))}
      </ul>
      {next?.chapter && (
        <div className="rounded-xl border bg-card p-4 text-left">
          <p className="text-xs text-muted-foreground">
            Up next · {next.trackTitle}
          </p>
          <p className="mt-0.5 font-medium">{next.chapterTitle}</p>
          <Button
            className="mt-3"
            onClick={() =>
              navigate({
                name: 'lesson',
                track: next.track,
                chapter: next.chapter ?? '',
                step: next.next?.ref,
              })
            }
          >
            Continue <ArrowRight />
          </Button>
        </div>
      )}
      <Button variant="ghost" onClick={() => navigate({ name: 'home' })}>
        Back to dashboard
      </Button>
    </div>
  );
}
