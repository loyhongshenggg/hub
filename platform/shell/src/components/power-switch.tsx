import { Switch } from '@hub/ui-kit';
import { isOn, useAppAction } from '../api/apps';
import type { AppInfo } from '../api/types';

export function PowerSwitch({ app }: { app: AppInfo }) {
  const action = useAppAction();
  const on = isOn(app);
  return (
    <Switch
      checked={on}
      disabled={app.status === 'stopping' || (!!app.removed && !on)}
      onCheckedChange={(next) =>
        action.mutate({ id: app.id, action: next ? 'start' : 'stop' })
      }
      aria-label={`Power ${app.name} ${on ? 'off' : 'on'}`}
      title={on ? 'Power off' : 'Power on'}
    />
  );
}
