// Everything finished, newest first, grouped by day.
import { Button } from '@hub/ui-kit';
import { RotateCcw, ThumbsDown, ThumbsUp } from 'lucide-react';
import type { HistoryEntry } from './api';
import { useHistory, useUndo } from './queries';
import { navigate } from './router';
import { Skeleton, TypeIcon, minutes } from './ui';

const ratingLabel = {
  easy: 'Too easy',
  right: 'Just right',
  hard: 'Too hard',
} as const;

function day(iso: string): string {
  const d = new Date(iso);
  const today = new Date();
  const y = new Date(today);
  y.setDate(today.getDate() - 1);
  if (d.toDateString() === today.toDateString()) return 'Today';
  if (d.toDateString() === y.toDateString()) return 'Yesterday';
  return d.toLocaleDateString(undefined, {
    weekday: 'long',
    month: 'short',
    day: 'numeric',
  });
}

export function History() {
  const q = useHistory();
  const undo = useUndo();
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  const groups: { day: string; items: HistoryEntry[] }[] = [];
  for (const it of items) {
    const d = day(it.at);
    const last = groups.at(-1);
    if (last?.day === d) last.items.push(it);
    else groups.push({ day: d, items: [it] });
  }
  return (
    <div className="mx-auto max-w-2xl space-y-6 pb-16">
      <h1 className="text-2xl font-semibold tracking-tight">History</h1>
      {q.isPending && <Skeleton className="h-64 rounded-xl" />}
      {!q.isPending && items.length === 0 && (
        <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
          Nothing finished yet. Steps you mark done land here.
        </p>
      )}
      {groups.map((g) => (
        <section key={g.day} className="space-y-2">
          <h2 className="text-sm font-semibold text-muted-foreground">
            {g.day} · {minutes(g.items.reduce((n, i) => n + i.minutes, 0))}
          </h2>
          <ul className="divide-y rounded-xl border bg-card">
            {g.items.map((it) => (
              <li
                key={it.ref}
                className="group flex items-center gap-3 px-4 py-3"
              >
                <TypeIcon
                  type={it.kind === 'practice' ? 'lab' : 'read'}
                  className="text-muted-foreground"
                />
                <button
                  className="min-w-0 flex-1 text-left"
                  onClick={() =>
                    it.track && it.chapter
                      ? navigate({
                          name: 'lesson',
                          track: it.track,
                          chapter: it.chapter,
                          step: it.ref,
                        })
                      : it.url &&
                        window.open(it.url, '_blank', 'noopener,noreferrer')
                  }
                >
                  <span className="line-clamp-1 text-sm font-medium">
                    {it.title}
                  </span>
                  <span className="line-clamp-1 text-xs text-muted-foreground">
                    {[
                      it.chapterTitle ?? it.trackTitle ?? it.by,
                      it.minutes ? minutes(it.minutes) : '',
                      it.rating
                        ? ratingLabel[it.rating as keyof typeof ratingLabel]
                        : '',
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                </button>
                {it.worth === true && (
                  <ThumbsUp className="size-3.5 text-muted-foreground" />
                )}
                {it.worth === false && (
                  <ThumbsDown className="size-3.5 text-muted-foreground" />
                )}
                <Button
                  variant="ghost"
                  size="icon-sm"
                  title="Undo"
                  aria-label="Undo"
                  className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                  onClick={() => undo.mutate(it.ref)}
                >
                  <RotateCcw />
                </Button>
              </li>
            ))}
          </ul>
        </section>
      ))}
      {q.hasNextPage && (
        <Button
          variant="outline"
          className="mx-auto flex"
          onClick={() => q.fetchNextPage()}
        >
          Load more
        </Button>
      )}
    </div>
  );
}
