import { Button, Card, CardContent, CardHeader, CardTitle } from '@hub/ui-kit';
import { LayoutGrid, Loader2, TriangleAlert } from 'lucide-react';
import { sortByName, useApps } from '../api/apps';
import { AppCard } from './app-card';

export function Home() {
  const { data, isPending, error, refetch } = useApps();

  if (isPending) {
    return (
      <div className="grid h-full place-items-center text-muted-foreground">
        <Loader2 className="size-6 animate-spin" />
      </div>
    );
  }
  if (error) {
    return (
      <div className="grid h-full place-items-center p-8 text-center">
        <div className="space-y-3">
          <p className="font-medium">Can't reach the hub core.</p>
          <p className="text-sm text-muted-foreground">{error.message}</p>
          <Button variant="outline" onClick={() => void refetch()}>
            Try again
          </Button>
        </div>
      </div>
    );
  }

  const apps = sortByName(data.apps);
  const running = apps.filter((a) => a.status === 'running').length;
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-6xl px-8 py-8">
        <header className="mb-6">
          <h1 className="text-2xl font-semibold tracking-tight">Apps</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {apps.length === 0
              ? 'Nothing installed yet'
              : `${running} of ${apps.length} running`}
          </p>
        </header>

        {data.problems.length > 0 && (
          <Card className="mb-6 gap-2 border-destructive/30 bg-destructive/5 py-4">
            <CardHeader className="px-4">
              <CardTitle className="flex items-center gap-2 text-sm text-destructive">
                <TriangleAlert className="size-4" />
                {data.problems.length === 1
                  ? '1 app has an invalid manifest'
                  : `${data.problems.length} apps have invalid manifests`}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 px-4">
              {data.problems.map((p) => (
                <div key={p.id} className="text-sm">
                  <span className="font-medium">{p.id}</span>
                  <pre className="mt-1 overflow-x-auto whitespace-pre-wrap rounded-md bg-background/60 p-2 font-mono text-xs text-muted-foreground">
                    {p.error}
                  </pre>
                </div>
              ))}
            </CardContent>
          </Card>
        )}

        {apps.length === 0 ? (
          <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-16 text-center">
            <LayoutGrid className="size-8 text-muted-foreground" />
            <p className="font-medium">No apps yet</p>
            <p className="max-w-sm text-sm text-muted-foreground">
              Add a folder under{' '}
              <code className="rounded bg-muted px-1 py-0.5">
                apps/&lt;id&gt;/
              </code>{' '}
              with an{' '}
              <code className="rounded bg-muted px-1 py-0.5">
                app.manifest.yaml
              </code>
              . It shows up here within a couple of seconds.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(17rem,1fr))] gap-4">
            {apps.map((a) => (
              <AppCard key={a.id} app={a} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
