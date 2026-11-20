import { Button, cn } from '@hub/ui-kit';
import { ArrowDownToLine, Eraser, X } from 'lucide-react';
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useApp } from '../api/apps';
import type { AppInfo, LogLine } from '../api/types';
import { useUI } from '../state/ui';
import { hue } from './app-icon';
import { StatusDot } from './status';

const MAX_LINES = 5000;

/** Bottom panel with live container logs of one app. */
export function LogsDrawer() {
  const id = useUI((s) => s.logsFor);
  const app = useApp(id);
  return app ? <LogsPanel key={app.id} app={app} /> : null;
}

function useLogStream(app: AppInfo) {
  const [lines, setLines] = useState<LogLine[]>([]);
  // Reconnect on every lifecycle change: containers come and go with it.
  const streamKey =
    app.status === 'stopping' ? 'hold' : `${app.status}:${app.since}`;
  useEffect(() => {
    if (streamKey === 'hold') return;
    const es = new EventSource(`/api/apps/${app.id}/logs?tail=500`);
    let buffer: LogLine[] = [];
    let fresh = true; // first batch replaces what a previous stream showed
    let frame = 0;
    const flush = () => {
      frame = 0;
      const batch = buffer;
      buffer = [];
      setLines((prev) => {
        const next = fresh ? batch : prev.concat(batch);
        fresh = false;
        return next.length > MAX_LINES ? next.slice(-MAX_LINES) : next;
      });
    };
    es.addEventListener('log', (e) => {
      buffer.push(JSON.parse((e as MessageEvent<string>).data) as LogLine);
      frame ||= requestAnimationFrame(flush);
    });
    // The stream ends when the containers go away; status changes reopen it.
    es.addEventListener('end', () => es.close());
    es.onerror = () => es.close();
    return () => {
      es.close();
      cancelAnimationFrame(frame);
    };
  }, [app.id, streamKey]);
  return [lines, setLines] as const;
}

function LogsPanel({ app }: { app: AppInfo }) {
  const showLogs = useUI((s) => s.showLogs);
  const [lines, setLines] = useLogStream(app);
  const [follow, setFollow] = useState(true);
  const box = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [lines, follow]);

  return (
    <section
      aria-label={`${app.name} logs`}
      className="flex h-72 shrink-0 flex-col border-t bg-card"
    >
      <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
        <StatusDot status={app.status} />
        <h2 className="text-sm font-medium">{app.name} logs</h2>
        <span className="text-xs text-muted-foreground">
          {lines.length} lines
        </span>
        <div className="ml-auto flex items-center gap-1">
          <Button
            size="icon-sm"
            variant={follow ? 'secondary' : 'ghost'}
            aria-label="Follow new lines"
            aria-pressed={follow}
            title="Follow new lines"
            onClick={() => setFollow((f) => !f)}
          >
            <ArrowDownToLine />
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="Clear"
            title="Clear"
            onClick={() => setLines([])}
          >
            <Eraser />
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="Close logs"
            title="Close"
            onClick={() => showLogs(null)}
          >
            <X />
          </Button>
        </div>
      </header>
      <div
        ref={box}
        onScroll={(e) => {
          const el = e.currentTarget;
          setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
        }}
        className="min-h-0 flex-1 overflow-y-auto px-3 py-2 font-mono text-xs leading-5"
      >
        {lines.length === 0 ? (
          <p className="text-muted-foreground">
            {app.status === 'stopped'
              ? 'The app is off — no containers, no logs.'
              : 'No log lines yet.'}
          </p>
        ) : (
          lines.map((l, i) => (
            <div key={i} className="flex gap-3 whitespace-pre-wrap break-all">
              <span
                className="w-16 shrink-0 truncate text-right"
                style={{ color: `oklch(0.65 0.13 ${hue(l.service)})` }}
              >
                {l.service}
              </span>
              <span className={cn(l.stream === 'stderr' && 'text-destructive')}>
                {l.text}
              </span>
            </div>
          ))
        )}
      </div>
    </section>
  );
}
