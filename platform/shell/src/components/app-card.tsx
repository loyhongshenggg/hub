import {
  Button,
  Card,
  CardAction,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@hub/ui-kit';
import {
  Check,
  ExternalLink,
  MoreHorizontal,
  Pin,
  PinOff,
  RotateCw,
  ScrollText,
} from 'lucide-react';
import { useAppAction, usePatchApp } from '../api/apps';
import type { AppInfo } from '../api/types';
import { useTabs } from '../state/tabs';
import { useUI } from '../state/ui';
import { AppIcon } from './app-icon';
import { PowerSwitch } from './power-switch';
import { statusLabel, StatusDot } from './status';

export function AppCard({ app }: { app: AppInfo }) {
  const openApp = useTabs((s) => s.openApp);
  const showLogs = useUI((s) => s.showLogs);
  const patch = usePatchApp();
  const action = useAppAction();
  return (
    <Card className="gap-3 py-4 transition-shadow hover:shadow-md">
      <CardHeader className="px-4">
        <div className="flex min-w-0 items-center gap-3">
          <AppIcon app={app} size={40} />
          <div className="min-w-0">
            <CardTitle className="truncate leading-tight">{app.name}</CardTitle>
            <div className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
              <StatusDot status={app.status} />
              {statusLabel[app.status]}
              {app.removed && ' · removed'}
            </div>
          </div>
        </div>
        <CardAction>
          <PowerSwitch app={app} />
        </CardAction>
      </CardHeader>
      <CardContent className="min-h-10 flex-1 px-4 text-sm text-muted-foreground">
        <p className="line-clamp-2">
          {app.description || (
            <span className="italic opacity-70">No description</span>
          )}
        </p>
        {app.status === 'error' && app.error && (
          <p
            className="mt-2 line-clamp-3 rounded-md bg-destructive/8 px-2 py-1.5 font-mono text-xs text-destructive"
            title={app.error}
          >
            {app.error}
          </p>
        )}
      </CardContent>
      <CardFooter className="gap-1.5 px-4">
        <Button
          size="sm"
          variant={app.status === 'running' ? 'default' : 'secondary'}
          onClick={() => openApp(app.id)}
        >
          Open
        </Button>
        <Button size="sm" variant="ghost" onClick={() => showLogs(app.id)}>
          <ScrollText /> Logs
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="icon-sm"
              variant="ghost"
              className="ml-auto"
              aria-label={`More actions for ${app.name}`}
            >
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onSelect={() =>
                patch.mutate({ id: app.id, prefs: { pinned: !app.pinned } })
              }
            >
              {app.pinned ? <PinOff /> : <Pin />}
              {app.pinned ? 'Unpin from dock' : 'Pin to dock'}
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() =>
                patch.mutate({
                  id: app.id,
                  prefs: { autoStart: !app.autoStart },
                })
              }
            >
              <Check className={app.autoStart ? '' : 'invisible'} />
              Start with the hub
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              disabled={app.status !== 'running' && app.status !== 'error'}
              onSelect={() => action.mutate({ id: app.id, action: 'restart' })}
            >
              <RotateCw /> Restart
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={app.status !== 'running'}
              onSelect={() =>
                window.open(app.url + app.uiPath, '_blank', 'noopener')
              }
            >
              <ExternalLink /> Open in a browser tab
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </CardFooter>
    </Card>
  );
}
