import { Badge } from '@hub/ui-kit';
import { useEffect, useState } from 'react';
import { api, type FetchStatus, type Source } from './api';
import { categoryColor, categoryName } from './categories';
import { ago, host } from './format';

export function Sources({ fetchStatus }: { fetchStatus: FetchStatus | null }) {
  const [sources, setSources] = useState<Source[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const lastRound = fetchStatus?.lastRound;
  useEffect(() => {
    api.sources().then(
      (r) => setSources(r.sources),
      (e: Error) => setError(e.message),
    );
  }, [lastRound]);

  if (error) return <p className="text-sm text-destructive">{error}</p>;
  if (!sources) return null;
  const active = sources.filter((s) => s.active);
  const failing = active.filter((s) => s.lastError).length;
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground">
        {active.length} feeds, refreshed every 30 minutes
        {failing > 0 && ` · ${failing} failing`}. Edit the list in{' '}
        <code className="rounded bg-muted px-1">
          apps/radar/api/content/sources.yaml
        </code>
        .
      </p>
      <div className="overflow-x-auto rounded-xl border bg-card">
        <table className="w-full text-sm">
          <thead className="border-b text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-4 py-2 font-medium">Source</th>
              <th className="px-4 py-2 font-medium">Category</th>
              <th className="px-4 py-2 text-right font-medium">Items</th>
              <th className="px-4 py-2 font-medium">Last success</th>
              <th className="px-4 py-2 font-medium">Status</th>
            </tr>
          </thead>
          <tbody>
            {active.map((s) => (
              <tr key={s.id} className="border-b last:border-b-0">
                <td className="px-4 py-2.5">
                  <div className="font-medium">{s.name}</div>
                  <div className="text-xs text-muted-foreground">
                    {host(s.url)}
                  </div>
                </td>
                <td className="px-4 py-2.5">
                  <span className="inline-flex items-center gap-1.5">
                    <span
                      className="size-2 rounded-full"
                      style={{ background: categoryColor(s.category) }}
                    />
                    {categoryName(s.category)}
                  </span>
                </td>
                <td className="px-4 py-2.5 text-right tabular-nums">
                  {s.items}
                </td>
                <td className="px-4 py-2.5 text-muted-foreground">
                  {ago(s.lastOkAt)}
                </td>
                <td className="max-w-64 px-4 py-2.5">
                  {s.lastError ? (
                    <span
                      className="line-clamp-2 text-xs text-destructive"
                      title={s.lastError}
                    >
                      {s.lastError}
                    </span>
                  ) : s.lastOkAt ? (
                    <Badge variant="success">OK</Badge>
                  ) : (
                    <Badge variant="secondary">pending</Badge>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
