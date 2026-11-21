// Shows a step, practice or item inside Radar in the mode the API picked:
// reader (clean article), video (nocookie embed), live (iframe) or outside
// (new tab, with a preview when the site allows it).
import { Button, cn } from '@hub/ui-kit';
import { ExternalLink, Eye, EyeOff, RotateCw } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { ReaderView } from './api';
import { useReader } from './queries';
import { Skeleton, useDelayed } from './ui';

export function hostOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return url;
  }
}

export function OpenOutside({
  url,
  label,
  variant = 'outline',
}: {
  url: string;
  label?: string;
  variant?: 'outline' | 'default' | 'ghost';
}) {
  return (
    <Button asChild variant={variant} size="sm">
      <a href={url} target="_blank" rel="noopener noreferrer">
        <ExternalLink /> {label ?? `Open ${hostOf(url)}`}
      </a>
    </Button>
  );
}

export function Viewer({
  refId,
  className,
  compact = false,
}: {
  refId: string;
  className?: string;
  /** Inside a lesson the step already shows the title: show only the source line. */
  compact?: boolean;
}) {
  const q = useReader(refId);
  const slow = useDelayed(q.isPending);
  if (q.isPending)
    return slow ? <ViewerSkeleton /> : <div className="min-h-64" />;
  if (q.isError)
    return (
      <div className="rounded-xl border border-dashed p-6 text-sm text-muted-foreground">
        <p>Couldn't load this here: {q.error.message}</p>
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={() => q.refetch()}
        >
          <RotateCw /> Try again
        </Button>
      </div>
    );
  return <ViewBody view={q.data} className={className} compact={compact} />;
}

function ViewerSkeleton() {
  return (
    <div className="max-w-[66ch] space-y-3" aria-busy="true">
      <Skeleton className="h-7 w-3/4" />
      <Skeleton className="h-4 w-1/3" />
      <div className="space-y-2 pt-4">
        {[100, 96, 99, 90, 97, 60].map((w, i) => (
          <Skeleton key={i} className="h-4" style={{ width: `${w}%` }} />
        ))}
      </div>
    </div>
  );
}

function ViewBody({
  view,
  className,
  compact,
}: {
  view: ReaderView;
  className?: string;
  compact: boolean;
}) {
  switch (view.mode) {
    case 'reader':
      return view.article ? (
        <Article
          view={view}
          article={view.article}
          className={className}
          compact={compact}
        />
      ) : null;
    case 'video':
      return (
        <div className={cn('space-y-3', className)}>
          <iframe
            src={view.embed}
            title="Video"
            className="aspect-video w-full rounded-xl border bg-muted"
            allow="accelerometer; encrypted-media; picture-in-picture; fullscreen"
            referrerPolicy="strict-origin-when-cross-origin"
            allowFullScreen
          />
          <OpenOutside
            url={view.url}
            label="Watch on YouTube"
            variant="ghost"
          />
        </div>
      );
    case 'live':
      return (
        <div className={cn('space-y-2', className)}>
          <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
            <span>{view.note ?? `Live from ${hostOf(view.url)}`}</span>
            <OpenOutside url={view.url} label="New tab" variant="ghost" />
          </div>
          <LiveFrame src={view.embed ?? view.url} />
        </div>
      );
    default:
      return <Outside view={view} className={className} />;
  }
}

function LiveFrame({ src }: { src: string }) {
  return (
    <iframe
      src={src}
      title="Live page"
      className="h-[min(78dvh,900px)] w-full rounded-xl border bg-background"
      sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads"
      referrerPolicy="no-referrer"
      loading="lazy"
    />
  );
}

function Outside({
  view,
  className,
}: {
  view: ReaderView;
  className?: string;
}) {
  const [preview, setPreview] = useState(false);
  return (
    <div className={cn('space-y-3', className)}>
      <div className="flex flex-col gap-3 rounded-xl border bg-card p-5 @lg:flex-row @lg:items-center">
        <div className="min-w-0 flex-1">
          <p className="font-medium">Opens in a new tab</p>
          <p className="mt-1 text-sm text-muted-foreground">{view.note}</p>
        </div>
        <div className="flex shrink-0 gap-2">
          {view.embed && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setPreview((p) => !p)}
            >
              {preview ? <EyeOff /> : <Eye />}{' '}
              {preview ? 'Hide preview' : 'Preview here'}
            </Button>
          )}
          <OpenOutside url={view.url} variant="default" />
        </div>
      </div>
      {preview && view.embed && <LiveFrame src={view.embed} />}
    </div>
  );
}

/** A sanitised article. YouTube placeholders become nocookie players. */
function Article({
  view,
  article: a,
  className,
  compact,
}: {
  view: ReaderView;
  article: NonNullable<ReaderView['article']>;
  className?: string;
  compact: boolean;
}) {
  const body = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = body.current;
    if (!el) return;
    el.querySelectorAll('img').forEach((img) => {
      img.loading = 'lazy';
      img.decoding = 'async';
      img.referrerPolicy = 'no-referrer';
    });
    el.querySelectorAll<HTMLElement>('figure[data-youtube]').forEach((fig) => {
      const id = fig.dataset.youtube ?? '';
      if (!/^[A-Za-z0-9_-]{11}$/.test(id) || fig.querySelector('iframe'))
        return;
      const f = document.createElement('iframe');
      f.src = `https://www.youtube-nocookie.com/embed/${id}`;
      f.title = 'Video';
      f.className = 'yt';
      f.allow =
        'accelerometer; encrypted-media; picture-in-picture; fullscreen';
      f.allowFullscreen = true;
      f.loading = 'lazy';
      f.referrerPolicy = 'strict-origin-when-cross-origin';
      fig.appendChild(f);
    });
  }, [a.html]);
  const meta = [a.byline, a.site || hostOf(view.url), `${a.minutes} min read`]
    .filter(Boolean)
    .join(' · ');
  return (
    <article className={cn('reader', className)}>
      <header className="!mt-0 space-y-1">
        {!compact && <h1 className="!mt-0 text-balance">{a.title}</h1>}
        <p className="flex flex-wrap items-center gap-x-3 text-sm text-muted-foreground">
          <span>{meta}</span>
          <a
            href={view.url}
            target="_blank"
            rel="noopener noreferrer"
            className="!text-muted-foreground !no-underline hover:!text-foreground"
          >
            Original <ExternalLink className="inline size-3.5 align-[-2px]" />
          </a>
        </p>
      </header>
      {/* Sanitised server-side (bluemonday UGC policy, no iframes or scripts). */}
      <div
        ref={body}
        className="reader-body"
        dangerouslySetInnerHTML={{ __html: a.html }}
      />
    </article>
  );
}
