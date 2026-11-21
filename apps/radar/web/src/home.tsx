// The dashboard: Learning (the main column, one primary action: Continue)
// and News (a side column of on-topic stories).
import { Button, cn } from '@hub/ui-kit';
import {
  ArrowRight,
  Clock,
  Compass,
  Newspaper,
  Sparkles,
  Trophy,
} from 'lucide-react';
import { ViewTransition } from 'react';
import type {
  Curriculum,
  Item,
  Recommendation,
  TrackSummary,
  Week,
} from './api';
import { categoryColor } from './categories';
import { ago } from './format';
import { useOpenItem } from './item-sheet';
import {
  useCurriculum,
  useDashboard,
  useFresh,
  useNewsPanel,
  useSnooze,
} from './queries';
import { navigate } from './router';
import {
  Difficulty,
  ProgressBar,
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
  return (
    <div className="@container">
      <div className="grid gap-x-10 gap-y-10 @3xl:grid-cols-[minmax(0,7fr)_minmax(16rem,3fr)]">
        <section aria-label="Learning" className="min-w-0 space-y-10">
          {dash.isError && (
            <p className="text-sm text-destructive">{dash.error.message}</p>
          )}
          {!dash.data && slow && <HomeSkeleton />}
          {dash.data && (
            <>
              {dash.data.needsAssessment && <AssessBanner />}
              <Continue rec={dash.data.continue} week={dash.data.week} />
              {dash.data.upNext.length > 0 && (
                <UpNext recs={dash.data.upNext} />
              )}
              <Tracks tracks={dash.data.tracks} cur={cur.data} />
              {dash.data.fresh > 0 && <Fresh />}
            </>
          )}
        </section>
        <aside
          aria-label="News"
          className="min-w-0 @3xl:sticky @3xl:top-4 @3xl:self-start"
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
      <Skeleton className="h-52 rounded-2xl" />
      <div className="grid gap-3 @xl:grid-cols-2">
        <Skeleton className="h-28 rounded-xl" />
        <Skeleton className="h-28 rounded-xl" />
      </div>
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
          {week.days.map((d, i) => (
            <span
              key={i}
              className={cn(
                'w-1.5 rounded-sm bg-primary/70',
                i === today && 'bg-primary',
                d === 0 && 'bg-muted',
              )}
              style={{ height: `${Math.max(3, (d / peak) * 24)}px` }}
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

function Continue({ rec, week }: { rec: Recommendation | null; week: Week }) {
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
    <div className="relative overflow-hidden rounded-2xl border bg-card shadow-xs">
      <div
        className="pointer-events-none absolute inset-0 bg-gradient-to-br from-primary/10 via-transparent to-transparent"
        aria-hidden="true"
      />
      <div className="relative space-y-5 p-5 @xl:p-7">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 space-y-1.5">
            <button
              onClick={() => navigate({ name: 'track', track: rec.track })}
              className="text-xs font-medium text-muted-foreground hover:text-foreground"
            >
              {rec.resume ? 'Continue' : 'Start'} · {rec.trackTitle}
              {rec.kind === 'chapter' &&
                ` · chapter ${rec.index + 1} of ${rec.count}`}
            </button>
            <ViewTransition
              name={
                rec.chapter ? `chapter-${rec.track}-${rec.chapter}` : undefined
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
                    className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground"
                  >
                    {w}
                  </span>
                ))}
              </p>
            )}
          </div>
          <WeekRing week={week} />
        </div>

        {rec.kind === 'chapter' && (
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <ProgressBar
              value={rec.stepsDone}
              max={rec.stepsTotal}
              className="max-w-56 flex-1"
            />
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
            <div className="flex min-w-0 flex-1 items-center gap-3 rounded-xl bg-muted/60 px-3.5 py-2.5">
              <TypeIcon
                type={next.type}
                kind={next.kind}
                className="text-muted-foreground"
              />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{next.title}</p>
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
          <Button size="lg" className="shrink-0" onClick={() => openRec(rec)}>
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
  );
}

function UpNext({ recs }: { recs: Recommendation[] }) {
  const snooze = useSnooze();
  return (
    <section>
      <SectionTitle>Up next</SectionTitle>
      <div className="grid gap-3 @xl:grid-cols-2">
        {recs.map((r) => (
          <div
            key={`${r.track}/${r.chapter ?? 'capstone'}`}
            className="group flex flex-col rounded-xl border bg-card p-4 transition-shadow duration-(--dur-base) hover:shadow-md"
          >
            <p className="truncate text-xs text-muted-foreground">
              {r.trackTitle}
            </p>
            <p className="mt-0.5 line-clamp-2 font-medium">{r.chapterTitle}</p>
            {r.why[0] && (
              <p className="mt-1 text-xs text-muted-foreground">{r.why[0]}</p>
            )}
            <div className="mt-auto flex items-center gap-2 pt-3">
              <Button size="sm" variant="secondary" onClick={() => openRec(r)}>
                Start <ArrowRight />
              </Button>
              {r.kind === 'chapter' && r.chapter && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-muted-foreground"
                  disabled={snooze.isPending}
                  onClick={() =>
                    r.chapter &&
                    snooze.mutate({ track: r.track, chapter: r.chapter })
                  }
                >
                  Not now
                </Button>
              )}
              <span className="ml-auto text-xs text-muted-foreground">
                {minutes(r.minutesLeft)}
              </span>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function mentorNames(ids: string[], cur?: Curriculum): string {
  return ids
    .map((id) => cur?.mentors.find((m) => m.id === id)?.name ?? id)
    .join(', ');
}

function Tracks({ tracks, cur }: { tracks: TrackSummary[]; cur?: Curriculum }) {
  const ready = tracks.filter((t) => !t.outline);
  const outlines = tracks.filter((t) => t.outline);
  return (
    <section>
      <SectionTitle>Tracks</SectionTitle>
      <div className="grid gap-3 @xl:grid-cols-2 @5xl:grid-cols-3">
        {ready.map((t) => (
          <button
            key={t.id}
            onClick={() => navigate({ name: 'track', track: t.id })}
            className="flex flex-col rounded-xl border bg-card p-4 text-left transition-[box-shadow,transform] duration-(--dur-base) ease-(--ease-out) hover:-translate-y-0.5 hover:shadow-md motion-reduce:hover:translate-y-0"
          >
            <ViewTransition name={`track-${t.id}`}>
              <p className="font-medium leading-snug">{t.title}</p>
            </ViewTransition>
            <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
              {t.tagline}
            </p>
            <div className="mt-auto pt-4">
              <ProgressBar value={t.completed} max={t.chapters} />
              <p className="mt-1.5 flex justify-between gap-2 text-xs text-muted-foreground">
                <span>
                  {t.status === 'done'
                    ? 'Completed'
                    : `${t.completed} of ${t.chapters} chapters`}
                </span>
                <span className="truncate">{mentorNames(t.mentors, cur)}</span>
              </p>
            </div>
          </button>
        ))}
      </div>
      {outlines.length > 0 && (
        <p className="mt-3 text-xs text-muted-foreground">
          Planned next: {outlines.map((t) => t.title.split(':')[0]).join(' · ')}
        </p>
      )}
    </section>
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
      <ul className="divide-y rounded-xl border bg-card">
        {items.map((it) => (
          <li key={it.id}>
            <button
              onClick={() => open(it)}
              className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors duration-(--dur-fast) hover:bg-accent/50"
            >
              <TypeIcon
                type={it.type ?? 'read'}
                className="text-muted-foreground"
              />
              <span className="min-w-0 flex-1">
                <span className="line-clamp-1 text-sm font-medium">
                  {it.title}
                </span>
                <span className="text-xs text-muted-foreground">
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
  const topicName = (id: string) =>
    cur?.topics.find((t) => t.id === id)?.name ?? id;
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
        {q.data?.items.map((it: Item) => (
          <li key={it.id}>
            <button
              onClick={() => open(it)}
              className="group w-full rounded-lg px-2.5 py-2 text-left transition-colors duration-(--dur-fast) hover:bg-accent/60"
            >
              <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <span
                  className="size-1.5 rounded-full"
                  style={{ background: categoryColor(it.source.category) }}
                />
                <span className="truncate">
                  {topicName(it.topics[0] ?? '')}
                </span>
                <span className="ml-auto shrink-0">{ago(it.publishedAt)}</span>
              </span>
              <span
                className={cn(
                  'mt-0.5 line-clamp-2 text-sm leading-snug',
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
        ))}
      </ul>
    </section>
  );
}
