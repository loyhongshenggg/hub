import type { HubClient, HubContext } from '@hub/app-sdk';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@hub/ui-kit';
import { Bell, ExternalLink, Hash, Type } from 'lucide-react';
import { useEffect, useState } from 'react';

interface Greeting {
  message: string;
  visits: number;
  appId: string;
}

export function App({ hub }: { hub: HubClient }) {
  const [greeting, setGreeting] = useState<Greeting | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [context, setContext] = useState<HubContext | null>(hub.context);
  const [theme, setTheme] = useState(hub.theme);
  const [badge, setBadge] = useState(0);
  const [title, setTitle] = useState('');
  const [target, setTarget] = useState('hello');

  useEffect(() => {
    fetch('/api/greeting')
      .then((r) =>
        r.ok
          ? r.json()
          : Promise.reject(new Error(`${r.status} ${r.statusText}`)),
      )
      .then(setGreeting, (e: Error) => setError(e.message));
  }, []);
  useEffect(() => hub.onContext(setContext), [hub]);
  useEffect(() => hub.onTheme(setTheme), [hub]);

  const bump = (n: number) => {
    setBadge(n);
    hub.setBadge(n);
  };

  return (
    <main className="mx-auto flex min-h-dvh max-w-3xl flex-col gap-6 px-6 py-10">
      <header className="flex items-center gap-4">
        <img src="/icon.svg" alt="" className="size-14" />
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {greeting?.message ?? 'Hello…'}
          </h1>
          <p className="text-sm text-muted-foreground">
            {error
              ? `API error: ${error}`
              : greeting
                ? `Visit #${greeting.visits} — the count lives in a Docker volume, so it survives power cycles.`
                : 'Asking the Go API…'}
          </p>
        </div>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Hub bridge</CardTitle>
          <CardDescription>
            What @hub/app-sdk knows about where this app runs.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
          <span className="text-muted-foreground">In the shell</span>
          <span>
            {hub.inShell ? (
              <Badge variant="success">yes</Badge>
            ) : (
              <Badge variant="secondary">no — opened directly</Badge>
            )}
          </span>
          <span className="text-muted-foreground">App id</span>
          <code>{context?.appId ?? '—'}</code>
          <span className="text-muted-foreground">Theme</span>
          <span>{theme}</span>
          <span className="text-muted-foreground">Locale</span>
          <span>{context?.locale ?? navigator.language}</span>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Talk to the shell</CardTitle>
          <CardDescription>
            {hub.inShell
              ? 'Each button sends one message to the hub.'
              : 'Open this app from the hub to try these.'}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              disabled={!hub.inShell}
              onClick={() => bump(badge + 1)}
            >
              <Hash /> Badge +1
            </Button>
            <Button
              variant="ghost"
              disabled={!hub.inShell || badge === 0}
              onClick={() => bump(0)}
            >
              Clear badge
            </Button>
            <Button
              variant="outline"
              disabled={!hub.inShell}
              onClick={() =>
                hub.notify(
                  'Hello says hi',
                  `Sent at ${new Date().toLocaleTimeString()}`,
                )
              }
            >
              <Bell /> Notify
            </Button>
          </div>
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              hub.setTitle(title.trim() || null);
            }}
          >
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Tab title (empty restores the name)"
              className="h-9 flex-1 rounded-md border bg-background px-3 text-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
            />
            <Button type="submit" variant="outline" disabled={!hub.inShell}>
              <Type /> Set title
            </Button>
          </form>
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              hub.openApp(target.trim());
            }}
          >
            <input
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder="App id"
              className="h-9 flex-1 rounded-md border bg-background px-3 text-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
            />
            <Button
              type="submit"
              variant="outline"
              disabled={!hub.inShell || !target.trim()}
            >
              <ExternalLink /> Open app
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
