// The dashboard. Learning (main column): today's pick (one primary action),
// then every track with its chapters, so any topic is one click away. News
// (side column): on-topic stories.
import { Button, cn } from '@hub/ui-kit';
import {
  ArrowRight,
  Check,
  Clock,
  Compass,
  Newspaper,
  Sparkles,
  Trophy,
} from 'lucide-react';
import { ViewTransition, useEffect, useRef } from 'react';
import type {
  Curriculum,
  Item,
  Recommendation,
  TrackSummary,
  Week,
} from './api';
import { ago } from './format';
import { useOpenItem } from './item-sheet';
import { AreaIcon, Face, Faces, tint } from './look';
import {
  useCurriculum,
  useDashboard,
  useFresh,
  useNewsPanel,
  useSnooze,
  useTrack,
} from './queries';
import { navigate } from './router';
import {
  Difficulty,
  Ring,
  SectionTitle,
  Skeleton,
  TypeIcon,
  minutes,
  useDelayed,
} from './ui';

export function Home() {
  const dash = useDashboard();
  const cur = useCurriculum();
  const slow = useDelayed(dash.isPending);
  const d = dash.data;
  const areaOf = (track: string) => d?.tracks.find((t) => t.id === track)?.area;
  return (
    <div className="@container">
      <div className="grid gap-x-10 gap-y-10 @3xl:grid-cols-[minmax(0,7fr)_minmax(16rem,3fr)]">
        <section aria-label="Learning" className="min-w-0 space-y-10">
          {dash.isError && (
            <p className="text-sm text-destructive">{dash.error.message}</p>
          )}
          {!d && slow && <HomeSkeleton />}
          {d && (
            <>
              {d.needsAssessment && <AssessBanner />}
              <Today
                rec={d.continue}
                week={d.week}
                area={d.continue ? areaOf(d.continue.track) : undefined}
              />
              <Tracks tracks={d.tracks} current={d.continue} />
              {d.fresh > 0 && <Fresh />}
            </>
          )}
        </section>
        <aside
          aria-label="News"
          className="min-w-0 @3xl:sticky @3xl:top-20 @3xl:self-start"
        >
          <NewsPanel cur={cur.data} />
        </aside>
      </div>
    </div>
  );
}

function HomeSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      <Skeleton className="h-56 rounded-2xl" />
      <Skeleton className="h-52 rounded-2xl" />
      <Skeleton className="h-52 rounded-2xl" />
    </div>
  );
}

function AssessBanner() {
  return (
    <button
      onClick={() => navigate({ name: 'assess' })}
      className="flex w-full items-center gap-3 rounded-xl border border-dashed px-4 py-3 text-left text-sm transition-colors duration-(--dur-fast) hover:bg-accent"
    >
      <Compass className="size-5 shrink-0 text-primary" />
      <span className="flex-1">
        <span className="font-medium">Tell Radar what you already know</span>
        <span className="text-muted-foreground">
          {' '}
          — one minute, so it starts at the right level.
        </span>
      </span>
      <ArrowRight className="size-4 text-muted-foreground" />
    </button>
  );
}

function WeekRing({ week }: { week: Week }) {
  const today = (new Date().getDay() + 6) % 7;
  const peak = Math.max(30, ...week.days);
  return (
    <div
      className="flex items-center gap-3"
      title={`${week.minutes} of ${week.goal} minutes this week`}
    >
      <Ring value={week.minutes} max={week.goal} size={48}>
        {Math.round((week.minutes / Math.max(1, week.goal)) * 100)}%
      </Ring>
      <div className="hidden @lg:block">
        <div className="flex h-6 items-end gap-0.5" aria-hidden="true">
          {week.days.map((m, i) => (
            <span
              key={i}
              className={cn(
                'w-1.5 rounded-sm bg-primary/60',
                i === today && 'bg-primary',
                m === 0 && 'bg-muted',
              )}
              style={{ height: `${Math.max(3, (m / peak) * 24)}px` }}
            />
          ))}
        </div>
        <p className="mt-1 text-xs text-muted-foreground tabular-nums">
          {minutes(week.minutes)} of {minutes(week.goal)}
        </p>
      </div>
    </div>
  );
}

function openRec(r: Recommendation) {
  if (r.kind === 'capstone' || !r.chapter)
    navigate({ name: 'track', track: r.track });
  else
    navigate({
      name: 'lesson',
      track: r.track,
      chapter: r.chapter,
      step: r.next?.ref,
    });
}

/** Today's pick: the one thing Radar suggests doing now. */
function Today({
  rec,
  week,
  area,
}: {
  rec: Recommendation | null;
  week: Week;
  area?: string;
}) {
  const snooze = useSnooze();
  if (!rec)
    return (
      <div className="rounded-2xl border bg-card p-8 text-center">
        <Trophy className="mx-auto size-8 text-success" />
        <p className="mt-3 font-semibold">Everything here is done</p>
        <p className="text-sm text-muted-foreground">
          New tracks and fresh mentor content will show up here.
        </p>
      </div>
    );
  const next = rec.next;
  return (
    <div
      style={tint(area)}
      className="relative overflow-hidden rounded-2xl border bg-card shadow-xs"
    >
      <div
        className="pointer-events-none absolute inset-0 bg-gradient-to-br from-(--tc)/18 via-(--tc)/5 to-transparent"
        aria-hidden="true"
      />
      <div
        className="pointer-events-none absolute -top-16 -right-16 size-56 rounded-full bg-(--tc)/12 blur-3xl"
        aria-hidden="true"
      />
      <div className="relative space-y-5 p-5 @xl:p-7">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 gap-4">
            <span className="hidden @md:block">
              <AreaIcon area={area} size="lg" />
            </span>
            <div className="min-w-0 space-y-1.5">
              <p className="flex flex-wrap items-center gap-x-2 text-xs font-medium">
                <span className="rounded-full bg-(--tc)/15 px-2 py-0.5 text-(--tc)">
                  {rec.resume ? 'Pick up where you left off' : "Today's pick"}
                </span>
                <button
                  onClick={() => navigate({ name: 'track', track: rec.track })}
                  className="text-muted-foreground hover:text-foreground"
                >
                  {rec.trackTitle.split(':')[0]}
                  {rec.kind === 'chapter' &&
                    ` · chapter ${rec.index + 1} of ${rec.count}`}
                </button>
              </p>
              <ViewTransition
                name={
                  rec.chapter
                    ? `chapter-${rec.track}-${rec.chapter}`
                    : undefined
                }
              >
                <h2 className="text-2xl font-semibold tracking-tight text-balance @xl:text-3xl">
                  {rec.chapterTitle}
                </h2>
              </ViewTransition>
              {rec.why.length > 0 && (
                <p className="flex flex-wrap gap-1.5 pt-1">
                  {rec.why.slice(0, 3).map((w) => (
                    <span
                      key={w}
                      className="rounded-full border border-(--tc)/25 px-2 py-0.5 text-xs text-muted-foreground"
                    >
                      {w}
                    </span>
                  ))}
                </p>
              )}
            </div>
          </div>
          <WeekRing week={week} />
        </div>

        {rec.kind === 'chapter' && (
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <div className="h-1.5 max-w-56 min-w-24 flex-1 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-(--tc) transition-[width] duration-(--dur-slow)"
                style={{
                  width: `${(rec.stepsDone / Math.max(1, rec.stepsTotal)) * 100}%`,
                }}
              />
            </div>
            <span className="tabular-nums">
              {rec.stepsDone}/{rec.stepsTotal} steps
            </span>
            <span className="flex items-center gap-1">
              <Clock className="size-3.5" /> {minutes(rec.minutesLeft)} left
            </span>
            {rec.difficulty ? <Difficulty level={rec.difficulty} /> : null}
          </div>
        )}

        <div className="flex flex-col gap-3 @lg:flex-row @lg:items-center">
          {next && (
            <div className="flex min-w-0 flex-1 items-center gap-3 rounded-xl bg-background/70 px-3.5 py-2.5 ring-1 ring-border">
              {next.byId ? (
                <Face id={next.byId} size="md" />
              ) : (
                <TypeIcon
                  type={next.type}
                  kind={next.kind}
                  className="text-muted-foreground"
                />
              )}
              <div className="min-w-0">
                <p className="flex items-center gap-1.5 truncate text-sm font-medium">
                  <TypeIcon
                    type={next.type}
                    kind={next.kind}
                    className="size-3.5 text-muted-foreground"
                  />
                  <span className="truncate">{next.title}</span>
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {[next.by, next.minutes ? minutes(next.minutes) : '']
                    .filter(Boolean)
                    .join(' · ') ||
                    (next.kind === 'checkpoint'
                      ? 'Check what you can do now'
                      : '')}
                </p>
              </div>
            </div>
          )}
          <div className="flex shrink-0 items-center gap-1">
            {!rec.resume && rec.kind === 'chapter' && rec.chapter && (
              <Button
                variant="ghost"
                className="text-muted-foreground"
                disabled={snooze.isPending}
                onClick={() =>
                  rec.chapter &&
                  snooze.mutate({ track: rec.track, chapter: rec.chapter })
                }
              >
                Not today
              </Button>
            )}
            <Button
              size="lg"
              className="bg-(--tc) text-white hover:bg-(--tc)/90"
              onClick={() => openRec(rec)}
            >
              {rec.kind === 'capstone'
                ? 'Open capstone'
                : rec.resume
                  ? 'Continue'
                  : 'Start'}
              <ArrowRight />
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function Tracks({
  tracks,
  current,
}: {
  tracks: TrackSummary[];
  current: Recommendation | null;
}) {
  const ready = tracks.filter((t) => !t.outline);
  const planned = tracks.filter((t) => t.outline);
  return (
    <section className="space-y-4">
      <SectionTitle>Your tracks</SectionTitle>
      {ready.map((t) => (
        <TrackRow
          key={t.id}
          summary={t}
          picked={current?.track === t.id ? current.chapter : undefined}
        />
      ))}
      {planned.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 pt-1 text-xs text-muted-foreground">
          <span>Coming next</span>
          {planned.map((t) => (
            <span
              key={t.id}
              style={tint(t.area)}
              className="inline-flex items-center gap-1.5 rounded-full border border-(--tc)/30 bg-(--tc)/8 py-0.5 pr-2.5 pl-1"
            >
              <AreaIcon area={t.area} size="sm" />
              {t.title.split(':')[0]}
            </span>
          ))}
        </div>
      )}
    </section>
  );
}

/** One track: header, then its chapters in a row you can scroll through. */
function TrackRow({
  summary: t,
  picked,
}: {
  summary: TrackSummary;
  picked?: string;
}) {
  const q = useTrack(t.id);
  const row = useRef<HTMLOListElement>(null);
  const chapters = q.data?.chapterList ?? [];
  const nextId = (
    chapters.find((c) => c.status === 'active') ??
    chapters.find((c) => c.status !== 'done')
  )?.id;

  // Start at the left. Scroll only if the next chapter would be off-screen,
  // and then keep the chapter before it in view for context. Positions are
  // measured against the row itself (offsetLeft is relative to the page).
  useEffect(() => {
    const list = row.current;
    const el = list?.querySelector<HTMLElement>('[data-next="true"]');
    if (!list || !el) return;
    const left =
      el.getBoundingClientRect().left -
      list.getBoundingClientRect().left +
      list.scrollLeft;
    if (left + el.offsetWidth <= list.clientWidth) {
      list.scrollLeft = 0;
      return;
    }
    const prev = el.previousElementSibling as HTMLElement | null;
    list.scrollLeft = Math.max(
      0,
      left - (prev ? prev.offsetWidth + 12 : 0) - 16,
    );
  }, [nextId]);

  return (
    <div
      style={tint(t.area)}
      className="overflow-hidden rounded-2xl border bg-card"
    >
      <button
        onClick={() => navigate({ name: 'track', track: t.id })}
        className="flex w-full items-center gap-3 bg-gradient-to-r from-(--tc)/14 via-(--tc)/4 to-transparent px-4 py-3.5 text-left transition-colors duration-(--dur-fast) hover:from-(--tc)/20"
      >
        <AreaIcon area={t.area} />
        <span className="min-w-0 flex-1">
          <ViewTransition name={`track-${t.id}`}>
            <span className="block truncate font-semibold">{t.title}</span>
          </ViewTransition>
          <span className="block truncate text-xs text-muted-foreground">
            {t.tagline}
          </span>
        </span>
        <span className="hidden flex-col items-end gap-1 @md:flex">
          <Faces ids={t.mentors} />
        </span>
        <span className="w-24 shrink-0 text-right">
          <span className="block text-xs font-medium tabular-nums">
            {t.status === 'done'
              ? 'Completed'
              : `${t.completed}/${t.chapters} chapters`}
          </span>
          <span className="mt-1 block h-1 overflow-hidden rounded-full bg-muted">
            <span
              className="block h-full rounded-full bg-(--tc)"
              style={{
                width: `${(t.completed / Math.max(1, t.chapters)) * 100}%`,
              }}
            />
          </span>
        </span>
      </button>
      <ol
        ref={row}
        className="flex snap-x gap-3 overflow-x-auto scroll-smooth px-4 pt-1 pb-4 [scrollbar-width:thin]"
      >
        {!q.data &&
          [0, 1, 2].map((i) => (
            <li key={i}>
              <Skeleton className="h-32 w-56 rounded-xl" />
            </li>
          ))}
        {chapters.map((c, i) => {
          const isNext = c.id === nextId;
          return (
            <li key={c.id} data-next={isNext} className="snap-start">
              <button
                onClick={() =>
                  navigate({ name: 'lesson', track: t.id, chapter: c.id })
                }
                className={cn(
                  'group relative flex h-full w-56 flex-col overflow-hidden rounded-xl border bg-background p-3 text-left transition-[box-shadow,transform] duration-(--dur-base) ease-(--ease-out) hover:-translate-y-0.5 hover:shadow-md motion-reduce:hover:translate-y-0',
                  isNext && 'border-(--tc)/60 ring-2 ring-(--tc)/25',
                  c.status === 'done' && 'opacity-75',
                )}
              >
                <span
                  className={cn(
                    'absolute inset-x-0 top-0 h-1',
                    c.status === 'new' ? 'bg-(--tc)/20' : 'bg-(--tc)',
                  )}
                />
                <span className="flex items-center justify-between gap-2 pt-1">
                  <span
                    className={cn(
                      'grid size-6 place-items-center rounded-full text-[11px] font-semibold tabular-nums',
                      c.status === 'done'
                        ? 'bg-(--tc) text-white'
                        : 'bg-(--tc)/12 text-(--tc)',
                    )}
                  >
                    {c.status === 'done' ? (
                      <Check className="size-3.5" strokeWidth={3} />
                    ) : (
                      i + 1
                    )}
                  </span>
                  {c.id === picked && (
                    <Sparkles
                      className="size-3.5 text-(--tc)"
                      aria-label="Today's pick"
                    />
                  )}
                </span>
                <span className="mt-2 line-clamp-2 text-sm font-medium leading-snug">
                  {c.title}
                </span>
                <span className="mt-auto flex items-center gap-2 pt-3 text-[11px] text-muted-foreground">
                  <Difficulty level={c.difficulty} />
                  <span>{minutes(c.minutes)}</span>
                  {c.status === 'active' && (
                    <span className="ml-auto font-medium text-(--tc) tabular-nums">
                      {c.stepsDone}/{c.stepsTotal}
                    </span>
                  )}
                  {isNext && c.status !== 'active' && (
                    <span className="ml-auto inline-flex items-center gap-0.5 font-medium text-(--tc)">
                      Start <ArrowRight className="size-3" />
                    </span>
                  )}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </div>
  );
}

function Fresh() {
  const q = useFresh();
  const open = useOpenItem();
  const items = q.data?.items ?? [];
  if (items.length === 0) return null;
  return (
    <section>
      <SectionTitle>
        <span className="inline-flex items-center gap-1.5">
          <Sparkles className="size-3.5" /> Fresh from your mentors
        </span>
      </SectionTitle>
      <ul className="grid gap-2 @xl:grid-cols-2">
        {items.map((it) => (
          <li key={it.id}>
            <button
              onClick={() => open(it)}
              className="flex h-full w-full items-center gap-3 rounded-xl border bg-card px-3.5 py-3 text-left transition-shadow duration-(--dur-base) hover:shadow-md"
            >
              <Face id={it.source.mentor} size="md" />
              <span className="min-w-0 flex-1">
                <span className="line-clamp-2 text-sm leading-snug font-medium">
                  {it.title}
                </span>
                <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground">
                  <TypeIcon type={it.type ?? 'read'} className="size-3" />
                  {it.source.name.split(' · ')[0]} · {ago(it.publishedAt)}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

function NewsPanel({ cur }: { cur?: Curriculum }) {
  const q = useNewsPanel();
  const open = useOpenItem();
  const topic = (id: string) => cur?.topics.find((t) => t.id === id);
  return (
    <section>
      <SectionTitle
        action={
          <button
            onClick={() => navigate({ name: 'news' })}
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            All news →
          </button>
        }
      >
        <span className="inline-flex items-center gap-1.5">
          <Newspaper className="size-3.5" /> On your topics
        </span>
      </SectionTitle>
      {!q.data && <Skeleton className="h-80 rounded-xl" />}
      {q.data && q.data.items.length === 0 && (
        <p className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
          No stories on your topics yet.
        </p>
      )}
      <ul className="space-y-1">
        {q.data?.items.map((it: Item) => {
          const tp = topic(it.topics[0] ?? '');
          return (
            <li key={it.id}>
              <button
                onClick={() => open(it)}
                className="group w-full rounded-lg px-2.5 py-2 text-left transition-colors duration-(--dur-fast) hover:bg-accent/60"
              >
                <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                  {tp && (
                    <span
                      style={tint(tp.area)}
                      className="truncate rounded-full bg-(--tc)/12 px-1.5 py-px font-medium text-(--tc)"
                    >
                      {tp.name}
                    </span>
                  )}
                  <span className="ml-auto shrink-0">
                    {ago(it.publishedAt)}
                  </span>
                </span>
                <span
                  className={cn(
                    'mt-1 line-clamp-2 text-sm leading-snug',
                    it.read ? 'text-muted-foreground' : 'font-medium',
                  )}
                >
                  {it.title}
                </span>
                <span className="text-[11px] text-muted-foreground">
                  {it.source.name}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
