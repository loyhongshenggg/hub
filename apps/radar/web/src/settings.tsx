// Learning settings: weekly goal, area priorities, time zone.
import { Button } from '@hub/ui-kit';
import { useState } from 'react';
import type { Settings as S } from './api';
import { useCurriculum, useSaveSettings, useSettings } from './queries';
import { navigate } from './router';
import { Segmented, Skeleton, minutes } from './ui';

const goals = [120, 180, 300, 480, 600];
const priority = [
  { value: '0', label: 'Off' },
  { value: '0.5', label: 'Low' },
  { value: '1', label: 'Normal' },
  { value: '2', label: 'High' },
];

export function Settings() {
  const q = useSettings();
  if (!q.data) return <Skeleton className="h-96 rounded-2xl" />;
  return <SettingsForm initial={q.data} />;
}

function SettingsForm({ initial }: { initial: S }) {
  const cur = useCurriculum();
  const save = useSaveSettings();
  const [s, setS] = useState<S>(initial);
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return (
    <div className="mx-auto max-w-2xl space-y-8 pb-16">
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>

      <section className="space-y-3">
        <h2 className="font-medium">Weekly goal</h2>
        <div className="flex flex-wrap gap-2">
          {goals.map((g) => (
            <Button
              key={g}
              size="sm"
              variant={s.weeklyMinutes === g ? 'default' : 'outline'}
              onClick={() => setS({ ...s, weeklyMinutes: g })}
            >
              {minutes(g)}
            </Button>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          Counted from the minutes of the steps you finish.
        </p>
      </section>

      <section className="space-y-3">
        <h2 className="font-medium">Priorities</h2>
        <p className="text-xs text-muted-foreground">
          How much each area should weigh when Radar picks your next chapter.
        </p>
        <ul className="divide-y rounded-xl border bg-card">
          {cur.data?.areas.map((a) => (
            <li
              key={a.id}
              className="flex items-center justify-between gap-3 px-4 py-3"
            >
              <span className="text-sm">{a.name}</span>
              <Segmented
                label={a.name}
                size="sm"
                options={priority}
                value={String(s.priorities[a.id] ?? 1)}
                onChange={(v) =>
                  setS({
                    ...s,
                    priorities: { ...s.priorities, [a.id]: Number(v || 1) },
                  })
                }
              />
            </li>
          ))}
        </ul>
      </section>

      <section className="space-y-2">
        <h2 className="font-medium">Time zone</h2>
        <p className="text-sm text-muted-foreground">
          Weeks start on Monday in {s.timezone || 'the server’s zone'}.
        </p>
        {s.timezone !== zone && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setS({ ...s, timezone: zone })}
          >
            Use this device’s zone ({zone})
          </Button>
        )}
      </section>

      <section className="space-y-2">
        <h2 className="font-medium">Starting level</h2>
        <Button
          variant="outline"
          size="sm"
          onClick={() => navigate({ name: 'assess' })}
        >
          Redo the self-check
        </Button>
      </section>

      <div className="flex items-center gap-3">
        <Button disabled={save.isPending} onClick={() => save.mutate(s)}>
          Save
        </Button>
        {save.isSuccess && <span className="text-sm text-success">Saved</span>}
        {save.isError && (
          <span className="text-sm text-destructive">{save.error.message}</span>
        )}
      </div>
    </div>
  );
}
