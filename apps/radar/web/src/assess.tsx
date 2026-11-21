// First-run self-check: how well you know each topic, so Radar starts at the
// right level. One tap per topic; skipping is fine.
import { Button } from '@hub/ui-kit';
import { useState } from 'react';
import type { Level } from './api';
import { useAssess, useAssessment, useCurriculum } from './queries';
import { navigate } from './router';
import { Segmented, Skeleton } from './ui';

const levelOptions = [
  { value: 'new' as const, label: 'New to me' },
  { value: 'some' as const, label: 'Some' },
  { value: 'solid' as const, label: 'Solid' },
];

export function Assess() {
  const q = useAssessment();
  const cur = useCurriculum();
  const save = useAssess();
  const [answers, setAnswers] = useState<Record<string, Level>>({});
  if (!q.data || !cur.data) return <Skeleton className="h-96 rounded-2xl" />;
  const value = (id: string): Level | '' =>
    answers[id] ?? q.data.topics.find((t) => t.id === id)?.level ?? '';
  const submit = (a: Record<string, Level>) =>
    save.mutate(a, { onSuccess: () => navigate({ name: 'home' }) });
  return (
    <div className="mx-auto max-w-2xl space-y-8 pb-16">
      <header className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">
          What do you already know?
        </h1>
        <p className="text-sm text-muted-foreground">
          A rough guess is enough. Radar uses it to pick where to start and
          adjusts as you go. Leave anything you're unsure about.
        </p>
      </header>
      {cur.data.areas.map((a) => (
        <section key={a.id} className="space-y-2">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase">
            {a.name}
          </h2>
          <ul className="divide-y rounded-xl border bg-card">
            {q.data.topics
              .filter((t) => t.area === a.id)
              .map((t) => (
                <li
                  key={t.id}
                  className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <span className="text-sm">{t.name}</span>
                  <Segmented
                    label={t.name}
                    options={levelOptions}
                    value={value(t.id)}
                    onChange={(v) =>
                      setAnswers((s) => {
                        const next = { ...s };
                        if (v) next[t.id] = v;
                        else delete next[t.id];
                        return next;
                      })
                    }
                    clearable
                    size="sm"
                  />
                </li>
              ))}
          </ul>
        </section>
      ))}
      <div className="sticky bottom-3 flex justify-end gap-2 rounded-2xl border bg-background/90 p-3 shadow-lg backdrop-blur">
        <Button variant="ghost" onClick={() => submit({})}>
          Skip for now
        </Button>
        <Button disabled={save.isPending} onClick={() => submit(answers)}>
          Save · {Object.keys(answers).length} topics
        </Button>
      </div>
    </div>
  );
}
