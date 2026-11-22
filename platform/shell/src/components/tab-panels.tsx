import { Button } from '@hub/ui-kit';
import { Code2, Loader2, Power, ScrollText } from 'lucide-react';
import { useState } from 'react';
import { useAppAction, useApps } from '../api/apps';
import type { AppInfo } from '../api/types';
import { useTabs } from '../state/tabs';
import { useUI } from '../state/ui';
import { AppIcon } from './app-icon';

/**
 * One panel per open tab. Running apps render as iframes; inactive ones stay
 * mounted (hidden) when the manifest asks for keepAlive, so their state
 * survives tab switches. Panels are rendered in open order, which only ever
 * appends or removes, so React never moves (= reloads) an iframe.
 */
export function TabPanels() {
  const { data } = useApps();
  const { open, active } = useTabs();
  return (
    <>
      {open.map((id) => {
        const app = data?.apps.find((a) => a.id === id);
        return app ? (
          <AppPanel key={id} app={app} active={active === id} />
        ) : null;
      })}
    </>
  );
}

function AppPanel({ app, active }: { app: AppInfo; active: boolean }) {
  if (app.status === 'running' && (active || app.keepAlive))
    return <AppFrame app={app} active={active} />;
  return active ? <OffPanel app={app} /> : null;
}

function AppFrame({ app, active }: { app: AppInfo; active: boolean }) {
  // src is fixed at mount: changing it would navigate (reload) the app.
  const [src] = useState(
    () => app.url + (useTabs.getState().takePath(app.id) ?? app.uiPath),
  );
  // An inactive frame stays laid out but invisible (not display:none), so
  // switching back only repaints it instead of laying the app out again.
  return (
    <iframe
      title={app.name}
      src={src}
      style={active ? undefined : { visibility: 'hidden' }}
      inert={!active}
      aria-hidden={active ? undefined : true}
      data-app-id={app.id}
      data-origin={app.url}
      allow="clipboard-read; clipboard-write; fullscreen"
      className="absolute inset-0 size-full border-0 bg-background"
    />
  );
}

const message: Record<AppInfo['status'], string> = {
  stopped: 'This app is off.',
  starting: 'Starting — building the image can take a while the first time.',
  stopping: 'Stopping…',
  error: 'This app failed.',
  running: '',
};

function OffPanel({ app }: { app: AppInfo }) {
  const action = useAppAction();
  const showLogs = useUI((s) => s.showLogs);
  const busy = app.status === 'starting' || app.status === 'stopping';
  return (
    <div className="absolute inset-0 grid place-items-center overflow-y-auto p-8">
      <div className="flex w-full max-w-lg flex-col items-center gap-3 text-center">
        <AppIcon app={app} size={64} />
        <h2 className="text-xl font-semibold">{app.name}</h2>
        <p className="text-sm text-muted-foreground">{message[app.status]}</p>
        {app.status === 'error' && app.error && (
          <pre className="max-h-48 w-full overflow-auto whitespace-pre-wrap rounded-md bg-destructive/8 p-3 text-left font-mono text-xs text-destructive">
            {app.error}
          </pre>
        )}
        <div className="mt-2 flex gap-2">
          {busy ? (
            <Button disabled>
              <Loader2 className="animate-spin" />{' '}
              {app.status === 'starting' ? 'Starting…' : 'Stopping…'}
            </Button>
          ) : (
            <>
              <Button
                disabled={app.removed}
                onClick={() => action.mutate({ id: app.id, action: 'start' })}
              >
                <Power /> Power on
              </Button>
              {app.devAvailable && (
                <Button
                  variant="outline"
                  disabled={app.removed}
                  title="Run on the host with live reload"
                  onClick={() =>
                    action.mutate({ id: app.id, action: 'start', mode: 'dev' })
                  }
                >
                  <Code2 /> Dev mode
                </Button>
              )}
            </>
          )}
          <Button variant="outline" onClick={() => showLogs(app.id)}>
            <ScrollText /> Logs
          </Button>
        </div>
      </div>
    </div>
  );
}
