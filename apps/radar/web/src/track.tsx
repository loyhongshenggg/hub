// A track: its chapters in order, progress, mentors, and the capstone.
import { Button, cn } from '@hub/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, Check, ExternalLink, Trophy } from 'lucide-react';
import { ViewTransition } from 'react';
import { api } from './api';
import { keys, useTrack } from './queries';
import { navigate } from './router';
import { Difficulty, ProgressBar, Skeleton, minutes, useDelayed } from './ui';

export function Track({ id }: { id: string }) {
  const q = useTrack(id);
  const qc = useQueryClient();
  const slow = useDelayed(q.isPending);
  if (q.isError)
    return <p className="text-sm text-destructive">{q.error.message}</p>;
  if (!q.data) return slow ? <Skeleton className="h-96 rounded-2xl" /> : null;
  const t = q.data;
  const firstOpen = t.chapterList.find((c) => c.status !== 'done');
  const capstoneReady = t.completed === t.chapters && t.capstone;
  return (
    <div className="@container mx-auto max-w-3xl space-y-8">
      <Button
        variant="ghost"
        size="sm"
        className="-ml-2"
        onClick={() => navigate({ name: 'home' })}
      >
        <ArrowLeft /> Dashboard
      </Button>
      <header className="space-y-3">
        <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t.level} · {t.chapters} chapters · {minutes(t.minutes)}
        </p>
        <ViewTransition name={`track-${t.id}`}>
          <h1 className="text-3xl font-semibold tracking-tight text-balance">
            {t.title}
          </h1>
        </ViewTransition>
        <p className="text-muted-foreground">{t.tagline}</p>
        <div className="flex items-center gap-3 pt-1">
          <ProgressBar
            value={t.completed}
            max={t.chapters}
            className="max-w-64 flex-1"
          />
          <span className="text-xs text-muted-foreground">
            {t.completed} of {t.chapters} done
          </span>
          {firstOpen && (
            <Button
              size="sm"
              className="ml-auto"
              onClick={() =>
                navigate({ name: 'lesson', track: t.id, chapter: firstOpen.id })
              }
            >
              {firstOpen.status === 'active'
                ? 'Continue'
                : t.completed
                  ? 'Next chapter'
                  : 'Start'}
            </Button>
          )}
        </div>
      </header>

      {t.mentorInfo.length > 0 && (
        <div className="flex flex-wrap gap-3">
          {t.mentorInfo.map((m) => (
            <div
              key={m.id}
              className="min-w-56 flex-1 rounded-xl border bg-card p-4 text-sm"
            >
              <p className="font-medium">{m.name}</p>
              <p className="text-xs text-muted-foreground">{m.role}</p>
              <p className="mt-2 text-muted-foreground">{m.focus}</p>
              <div className="mt-2 flex flex-wrap gap-x-3 text-xs">
                {m.links.map((l) => (
                  <a
                    key={l.url}
                    href={l.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline"
                  >
                    {l.label} <ExternalLink className="inline size-3" />
                  </a>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      <ol className="relative space-y-2">
        {t.chapterList.map((c, i) => (
          <li key={c.id}>
            <button
              onClick={() =>
                navigate({ name: 'lesson', track: t.id, chapter: c.id })
              }
              className="flex w-full items-start gap-4 rounded-xl border bg-card p-4 text-left transition-[box-shadow] duration-(--dur-base) hover:shadow-md"
            >
              <span
                className={cn(
                  'grid size-8 shrink-0 place-items-center rounded-full border text-sm font-medium tabular-nums',
                  c.status === 'done' && 'border-success bg-success text-white',
                  c.status === 'active' && 'border-primary text-primary',
                )}
              >
                {c.status === 'done' ? (
                  <Check className="size-4" strokeWidth={3} />
                ) : (
                  i + 1
                )}
              </span>
              <span className="min-w-0 flex-1">
                <ViewTransition name={`chapter-${t.id}-${c.id}`}>
                  <span className="block font-medium">{c.title}</span>
                </ViewTransition>
                <span className="mt-0.5 line-clamp-2 block text-sm text-muted-foreground">
                  {c.summary}
                </span>
                <span className="mt-2 flex items-center gap-3 text-xs text-muted-foreground">
                  <Difficulty level={c.difficulty} />
                  <span>{minutes(c.minutes)}</span>
                  {c.status === 'active' && (
                    <span>
                      {c.stepsDone}/{c.stepsTotal} steps
                    </span>
                  )}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ol>

      {t.capstone && (
        <section
          className={cn(
            'rounded-2xl border p-5',
            capstoneReady ? 'bg-card' : 'bg-muted/40',
          )}
        >
          <p className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <Trophy className="size-4" /> Capstone
          </p>
          <h2 className="mt-1 text-lg font-semibold">{t.capstone.title}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t.capstone.summary}
          </p>
          <ul className="mt-3 list-disc space-y-1 pl-5 text-sm">
            {t.capstone.steps.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ul>
          <div className="mt-4">
            {t.capstoneDone ? (
              <p className="flex items-center gap-2 text-sm text-success">
                <Check className="size-4" /> Done — track complete
              </p>
            ) : (
              <Button
                disabled={!capstoneReady}
                onClick={() =>
                  void api.done(`capstone:${t.id}`, '', true).then(() => {
                    void qc.invalidateQueries({ queryKey: keys.track(t.id) });
                    void qc.invalidateQueries({ queryKey: keys.dashboard });
                  })
                }
              >
                <Check />{' '}
                {capstoneReady
                  ? 'Mark capstone done'
                  : 'Unlocks after the last chapter'}
              </Button>
            )}
          </div>
        </section>
      )}
    </div>
  );
}
