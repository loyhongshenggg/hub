import { Button, cn, Input } from '@hub/ui-kit';
import {
  Bookmark,
  BookmarkCheck,
  CheckCheck,
  Circle,
  CircleCheck,
  Loader2,
  RefreshCw,
  Search,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import {
  api,
  type Counts,
  type FetchStatus,
  type Item,
  type NewsFilter,
} from './api';
import { categories, categoryColor } from './categories';
import { ago } from './format';
import { useOpenItem } from './item-sheet';

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

export function News({
  counts,
  fetchStatus,
  onChange,
}: {
  counts: Counts | null;
  fetchStatus: FetchStatus | null;
  onChange: () => void;
}) {
  const [category, setCategory] = useState<NewsFilter['category']>('all');
  const [search, setSearch] = useState('');
  const [unread, setUnread] = useState(false);
  const [saved, setSaved] = useState(false);
  const q = useDebounced(search, 250);
  const openItem = useOpenItem();
  const filter: NewsFilter = { category, q, unread, saved };
  const key = JSON.stringify(filter);

  // The first page is keyed by the filter and the last fetch round, so new
  // rounds reload it. State is only set from async results (no sync setState
  // in effects); "loading" is derived from which key the page belongs to.
  const pageKey = `${key}|${fetchStatus?.lastRound ?? ''}`;
  const [page, setPage] = useState<{
    key: string;
    items: Item[];
    cursor: string;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const loading = page?.key !== pageKey || loadingMore;
  const items = page?.items ?? [];
  const cursor = page?.cursor ?? '';

  useEffect(() => {
    let cancelled = false;
    api.news(JSON.parse(key) as NewsFilter).then(
      (res) => {
        if (cancelled) return;
        setPage({ key: pageKey, items: res.items, cursor: res.nextCursor });
        setError(null);
      },
      (e: Error) => {
        if (!cancelled) setError(e.message);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [key, pageKey]);

  const loadMore = async () => {
    if (!page) return;
    setLoadingMore(true);
    try {
      const res = await api.news(JSON.parse(key) as NewsFilter, page.cursor);
      setPage(
        (p) =>
          p && {
            ...p,
            items: [...p.items, ...res.items],
            cursor: res.nextCursor,
          },
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoadingMore(false);
    }
  };

  const setItems = (fn: (prev: Item[]) => Item[]) =>
    setPage((p) => p && { ...p, items: fn(p.items) });

  const update = (id: number, patch: Partial<Item>) =>
    setItems((prev) =>
      prev.map((it) => (it.id === id ? { ...it, ...patch } : it)),
    );

  const toggleRead = async (it: Item, read = !it.read) => {
    if (it.read === read) return;
    update(it.id, { read });
    try {
      await api.setRead(it.id, read);
      onChange();
    } catch {
      update(it.id, { read: it.read });
    }
  };
  const toggleSaved = async (it: Item) => {
    update(it.id, { saved: !it.saved });
    try {
      await api.setSaved(it.id, !it.saved);
      onChange();
    } catch {
      update(it.id, { saved: it.saved });
    }
  };
  const readAll = async () => {
    await api.readAll(filter);
    setItems((prev) => prev.map((it) => ({ ...it, read: true })));
    onChange();
  };

  const unreadIn = (c: NewsFilter['category']) =>
    (c === 'all' ? counts?.unread : counts?.unreadByCategory[c]) ?? 0;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <div
          role="tablist"
          aria-label="Category"
          className="flex rounded-lg bg-muted p-1"
        >
          {[{ id: 'all' as const, name: 'All', color: '' }, ...categories].map(
            (c) => (
              <button
                key={c.id}
                role="tab"
                aria-selected={category === c.id}
                onClick={() => setCategory(c.id)}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-3 py-1 text-sm font-medium text-muted-foreground transition-colors',
                  category === c.id &&
                    'bg-background text-foreground shadow-xs',
                )}
              >
                {c.color && (
                  <span
                    className="size-2 rounded-full"
                    style={{ background: c.color }}
                  />
                )}
                {c.name}
                {unreadIn(c.id) > 0 && (
                  <span className="text-xs tabular-nums opacity-70">
                    {unreadIn(c.id)}
                  </span>
                )}
              </button>
            ),
          )}
        </div>
        <div className="relative min-w-48 flex-1">
          <Search className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search titles and summaries"
            className="pl-8"
          />
        </div>
        <Button
          variant={unread ? 'secondary' : 'ghost'}
          size="sm"
          aria-pressed={unread}
          onClick={() => setUnread((v) => !v)}
        >
          <Circle /> Unread
        </Button>
        <Button
          variant={saved ? 'secondary' : 'ghost'}
          size="sm"
          aria-pressed={saved}
          onClick={() => setSaved((v) => !v)}
        >
          <Bookmark /> Saved{' '}
          {counts?.saved ? (
            <span className="tabular-nums opacity-70">{counts.saved}</span>
          ) : null}
        </Button>
      </div>

      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span>
          {fetchStatus?.running
            ? 'Fetching feeds…'
            : `Updated ${ago(fetchStatus?.lastRound)}`}
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label="Refresh feeds"
          title="Refresh feeds"
          onClick={() => void api.refresh().then(onChange)}
        >
          <RefreshCw className={cn(fetchStatus?.running && 'animate-spin')} />
        </Button>
        <a href="#/sources" className="hover:text-foreground hover:underline">
          Sources
        </a>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto h-7"
          disabled={unreadIn(category) === 0}
          onClick={() => void readAll()}
        >
          <CheckCheck /> Mark all read
        </Button>
      </div>

      {error && (
        <p className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </p>
      )}

      <div className="overflow-hidden rounded-xl border bg-card">
        {items.length === 0 && !loading && (
          <p className="px-4 py-12 text-center text-sm text-muted-foreground">
            {fetchStatus?.running || !fetchStatus?.lastRound
              ? 'Fetching feeds for the first time…'
              : 'Nothing here.'}
          </p>
        )}
        {items.map((it) => (
          <article
            key={it.id}
            className="group flex gap-3 border-b px-4 py-3 last:border-b-0 hover:bg-accent/40"
          >
            <span
              className="mt-2 size-2 shrink-0 rounded-full"
              style={{ background: categoryColor(it.source.category) }}
            />
            <div className="min-w-0 flex-1">
              <a
                href={it.url}
                target="_blank"
                rel="noopener noreferrer"
                onClick={(e) => {
                  // Plain click reads it inside Radar; modified clicks open the site.
                  if (
                    !(e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0)
                  ) {
                    e.preventDefault();
                    openItem(it);
                  }
                  void toggleRead(it, true);
                }}
                className={cn(
                  'leading-snug hover:underline',
                  it.read ? 'text-muted-foreground' : 'font-medium',
                )}
              >
                {it.title}
              </a>
              <div className="mt-0.5 text-xs text-muted-foreground">
                {it.source.name} · {ago(it.publishedAt)}
              </div>
              {it.summary && (
                <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
                  {it.summary}
                </p>
              )}
            </div>
            <div
              className={cn(
                'flex shrink-0 items-start gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100',
                it.saved && 'opacity-100',
              )}
            >
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={it.saved ? 'Unsave' : 'Save'}
                title={it.saved ? 'Unsave' : 'Save for later'}
                onClick={() => void toggleSaved(it)}
              >
                {it.saved ? (
                  <BookmarkCheck className="text-primary" />
                ) : (
                  <Bookmark />
                )}
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={it.read ? 'Mark unread' : 'Mark read'}
                title={it.read ? 'Mark unread' : 'Mark read'}
                onClick={() => void toggleRead(it)}
              >
                {it.read ? <CircleCheck /> : <Circle />}
              </Button>
            </div>
          </article>
        ))}
        {loading && (
          <div className="flex justify-center py-6 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        )}
      </div>
      {cursor && !loading && (
        <Button
          variant="outline"
          className="self-center"
          onClick={() => void loadMore()}
        >
          Load more
        </Button>
      )}
    </div>
  );
}
