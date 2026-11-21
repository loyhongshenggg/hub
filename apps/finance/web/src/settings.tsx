import { Button, Input, Switch } from '@hub/ui-kit';
import { Database, Download, KeyRound, Loader2, Map as MapIcon, RefreshCw, Sparkles, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { api, type DirectorySource } from './api';
import { relTime } from './format';
import { keys, queryClient, useCategories, useClaude, useRules, useStatus } from './queries';
import { ErrorNote, Field, Meter, Section } from './ui';

export function SettingsPage() {
  return (
    <div className="flex flex-col gap-5">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
      <ClaudeSettings />
      <Directory />
      <Rules />
      <Accounts />
      <Section title="Your data">
        <p className="mb-3 text-sm text-muted-foreground">
          Everything lives in this app's own volume on your computer: the database, and the statement PDFs you imported.
        </p>
        <Button variant="outline" size="sm" asChild>
          <a href="/api/export.csv">
            <Download className="size-4" /> Export all transactions (CSV)
          </a>
        </Button>
      </Section>
    </div>
  );
}

function ClaudeSettings() {
  const { data: c } = useClaude();
  const [key, setKey] = useState('');
  const [cap, setCap] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [lookup, setLookup] = useState<string | null>(null);
  if (!c) return null;
  const save = async (p: Parameters<typeof api.saveClaude>[0]) => {
    setErr(null);
    setBusy(true);
    try {
      const v = await api.saveClaude(p);
      queryClient.setQueryData(keys.claude, v);
      void queryClient.invalidateQueries({ queryKey: keys.status });
    } catch (e) {
      setErr(e);
    } finally {
      setBusy(false);
    }
  };
  const spent = c.usage.costMicros / 1_000_000;
  const capUsd = c.capCents / 100;

  return (
    <Section title={<span className="flex items-center gap-2"><Sparkles className="size-4 text-[var(--seq)]" /> Claude (optional)</span>}>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-muted-foreground">
          Everything in Finance works without this. With a key, Claude ({c.model}) can read looser goal paragraphs and identify merchants the local directory can’t. Each feature has its own switch. Sent to Anthropic: goal paragraphs (with category averages if you allow it) and the text of
          merchants we couldn't identify. Never sent: statements, amounts, dates, your name or card numbers.
        </p>
        <Field
          label="Anthropic API key"
          hint={
            c.keySet
              ? c.keySource === 'env'
                ? `Using ANTHROPIC_API_KEY from the environment (${c.keyHint}).`
                : `Saved in this app (${c.keyHint}).`
              : 'Create one at console.anthropic.com. Stored only in this app’s local database.'
          }
        >
          <div className="flex gap-2">
            <Input type="password" autoComplete="off" placeholder={c.keySet ? '••••••••' : 'sk-ant-…'} value={key} onChange={(e) => setKey(e.target.value)} />
            <Button onClick={() => void save({ apiKey: key }).then(() => setKey(''))} disabled={busy || key.length < 10}>
              <KeyRound className="size-4" /> Save
            </Button>
            {c.keySource === 'settings' && (
              <Button variant="ghost" onClick={() => void save({ apiKey: '' })} disabled={busy}>
                Remove
              </Button>
            )}
          </div>
        </Field>
        <Toggle
          label="Turn a paragraph into goals"
          hint="On the Goals page: describe what you want; Claude drafts structured goals for you to check before saving."
          checked={c.goals}
          disabled={!c.keySet || busy}
          onChange={(v) => void save({ goals: v })}
        />
        <Toggle
          label="Share average spending with goal drafts"
          hint="Category averages only, so limits are realistic. You can untick it per paragraph."
          checked={c.shareAverages}
          disabled={!c.keySet || busy}
          onChange={(v) => void save({ shareAverages: v })}
        />
        <Toggle
          label="Identify unknown merchants"
          hint="After each import, merchants the local directory couldn't match are looked up with web search (Singapore). Answers are cached."
          checked={c.merchants}
          disabled={!c.keySet || busy}
          onChange={(v) => void save({ merchants: v })}
        />
        {c.merchants && (
          <Button
            variant="outline"
            size="sm"
            className="self-start"
            onClick={async () => {
              setLookup('…');
              try {
                const r = await api.lookupMerchants();
                setLookup(`Updated ${r.updated} transactions.`);
                void queryClient.invalidateQueries();
              } catch (e) {
                setLookup(e instanceof Error ? e.message : String(e));
              }
            }}
          >
            <Sparkles className="size-4" /> Look up unknown merchants now
          </Button>
        )}
        {lookup && <p className="text-sm text-muted-foreground">{lookup}</p>}
        <div className="grid gap-2 sm:grid-cols-[1fr_12rem] sm:items-end">
          <div>
            <div className="mb-1 flex justify-between text-sm">
              <span>This month</span>
              <span className="tabular text-muted-foreground">
                US${spent.toFixed(2)} of US${capUsd.toFixed(2)} · {c.usage.calls} calls
              </span>
            </div>
            <Meter value={capUsd > 0 ? spent / capUsd : 0} tone={spent >= capUsd ? 'bad' : 'seq'} label="Claude spend this month" />
          </div>
          <Field label="Monthly cap (US$)">
            <div className="flex gap-2">
              <Input inputMode="decimal" value={cap ?? String(capUsd)} onChange={(e) => setCap(e.target.value)} />
              <Button
                variant="outline"
                disabled={cap === null || busy}
                onClick={() => void save({ capCents: Math.round(parseFloat(cap ?? '0') * 100) }).then(() => setCap(null))}
              >
                Set
              </Button>
            </div>
          </Field>
        </div>
        <ErrorNote error={err} />
      </div>
    </Section>
  );
}

function Toggle({ label, hint, checked, disabled, onChange }: { label: string; hint: string; checked: boolean; disabled?: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-start justify-between gap-4 text-sm">
      <span>
        <span className="font-medium">{label}</span>
        <span className="block text-xs text-muted-foreground">{hint}</span>
      </span>
      <Switch checked={checked} disabled={disabled} onCheckedChange={onChange} />
    </label>
  );
}

const sourceInfo: Record<string, { name: string; icon: typeof MapIcon; text: string; button: string }> = {
  osm: {
    name: 'OpenStreetMap — Singapore shops & food places',
    icon: MapIcon,
    text: '~20k named places with brand, branch and type (© OpenStreetMap contributors, ODbL). A few MB; refreshed on demand.',
    button: 'Refresh',
  },
  acra: {
    name: 'ACRA — registered businesses',
    icon: Database,
    text: 'Live Singapore businesses in consumer industries, with their SSIC industry code (data.gov.sg, Open Data Licence). Matches legal names like “GAO JI FOOD (S) PTE. LTD.”. Larger: a few minutes to download.',
    button: 'Download',
  },
};

function Directory() {
  const { data: status } = useStatus();
  const [err, setErr] = useState<unknown>(null);
  const bySource = new Map<string, DirectorySource>((status?.directory ?? []).map((d) => [d.source, d]));
  return (
    <Section title="Merchant directory">
      <p className="mb-3 text-sm text-muted-foreground">
        Card statements show short, cut-off names. These open datasets are downloaded and matched on this computer — your spending isn’t sent anywhere.
      </p>
      <ul className="flex flex-col gap-3">
        {(['osm', 'acra'] as const).map((src) => {
          const d = bySource.get(src);
          const info = sourceInfo[src];
          const Icon = info.icon;
          const running = d?.status === 'running';
          return (
            <li key={src} className="flex items-start gap-3 rounded-lg border p-3">
              <Icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium">{info.name}</div>
                <div className="text-xs text-muted-foreground">{info.text}</div>
                <div className="mt-1 text-xs">
                  {running ? (
                    <span className="flex items-center gap-1">
                      <Loader2 className="size-3 animate-spin" /> Downloading… {d?.error}
                    </span>
                  ) : d?.status === 'failed' ? (
                    <span className="text-[var(--bad)]">Failed: {d.error}</span>
                  ) : d?.count ? (
                    <span className="text-muted-foreground">
                      {d.count.toLocaleString()} entries · updated {relTime(d.refreshedAt)}
                    </span>
                  ) : (
                    <span className="text-muted-foreground">Not downloaded</span>
                  )}
                </div>
              </div>
              <Button
                variant="outline"
                size="sm"
                disabled={running}
                onClick={async () => {
                  setErr(null);
                  try {
                    await api.refreshDirectory(src);
                    void queryClient.invalidateQueries({ queryKey: keys.status });
                  } catch (e) {
                    setErr(e);
                  }
                }}
              >
                <RefreshCw className="size-4" /> {info.button}
              </Button>
            </li>
          );
        })}
      </ul>
      <ErrorNote error={err} />
    </Section>
  );
}

function Rules() {
  const { data: rules = [] } = useRules();
  const cats = useCategories();
  return (
    <Section title="Your rules">
      {rules.length === 0 ? (
        <p className="text-sm text-muted-foreground">When you fix a merchant and tick “apply to every…”, the fix is saved here and used for future imports.</p>
      ) : (
        <ul className="divide-y text-sm">
          {rules.map((r) => (
            <li key={r.key} className="flex items-center gap-3 py-2">
              <code className="w-48 truncate text-xs text-muted-foreground">{r.key}</code>
              <span className="flex-1 truncate">→ {r.merchant}</span>
              <span className="text-xs text-muted-foreground">{cats.get(r.category).name}</span>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`Delete rule for ${r.merchant}`}
                onClick={async () => {
                  await api.deleteRule(r.key);
                  void queryClient.invalidateQueries({ queryKey: keys.rules });
                }}
              >
                <Trash2 className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}

function Accounts() {
  const { data: status } = useStatus();
  const accounts = status?.accounts ?? [];
  if (accounts.length === 0) return null;
  return (
    <Section title="Accounts">
      <ul className="flex flex-col gap-2">
        {accounts.map((a) => (
          <li key={a.id} className="flex items-center gap-2">
            <Input
              defaultValue={a.name}
              className="max-w-sm"
              aria-label="Account name"
              onBlur={async (e) => {
                if (e.target.value && e.target.value !== a.name) {
                  await api.renameAccount(a.id, e.target.value);
                  void queryClient.invalidateQueries({ queryKey: keys.status });
                }
              }}
            />
            <span className="text-xs text-muted-foreground">
              {a.institution} · ··{a.last4} · {a.currency}
            </span>
          </li>
        ))}
      </ul>
      <p className="mt-2 text-xs text-muted-foreground">Accounts are created from statements; rename them here.</p>
    </Section>
  );
}
