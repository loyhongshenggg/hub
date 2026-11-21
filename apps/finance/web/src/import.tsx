import { Button, cn } from '@hub/ui-kit';
import { ArrowLeft, CheckCircle2, FileText, FileUp, Loader2, RefreshCw, ShieldCheck, Trash2, TriangleAlert, XCircle } from 'lucide-react';
import { useRef, useState } from 'react';
import { api, type Statement, type Txn } from './api';
import { date, money, plural } from './format';
import { useInvalidateLedger, useStatement, useStatements, useStatus } from './queries';
import { go, href, type Route } from './router';
import { TxnList, TxnSheet } from './sheets';
import { Empty, ErrorNote, Money, Section } from './ui';

export function ImportPage({ route }: { route: Route }) {
  if (route.id) return <StatementDetail id={Number(route.id)} />;
  return <ImportHome />;
}

function ImportHome() {
  const { data: statements = [] } = useStatements();
  const { data: status } = useStatus();
  const inv = useInvalidateLedger();
  const [drag, setDrag] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notes, setNotes] = useState<string[]>([]);
  const [err, setErr] = useState<unknown>(null);
  const input = useRef<HTMLInputElement>(null);

  const send = async (files: File[]) => {
    const pdfs = files.filter((f) => f.type === 'application/pdf' || f.name.toLowerCase().endsWith('.pdf'));
    if (pdfs.length === 0) return;
    setBusy(true);
    setErr(null);
    try {
      const r = await api.upload(pdfs);
      setNotes(
        r.uploads
          .filter((u) => u.duplicate || u.error)
          .map((u) => (u.error ? `${u.fileName}: ${u.error}` : `${u.fileName} was already imported.`)),
      );
      inv();
    } catch (e) {
      setErr(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <h1 className="text-xl font-semibold tracking-tight">Import statements</h1>
      {status && !status.ocr.ok && (
        <div className="flex items-start gap-2 rounded-lg bg-[var(--warn)]/15 px-3 py-2 text-sm">
          <TriangleAlert className="mt-0.5 size-4 shrink-0" />
          <span>
            Statement reading is unavailable: {status.ocr.error}. In Dev mode install them once with{' '}
            <code className="rounded bg-muted px-1">sudo apt install poppler-utils tesseract-ocr</code>; the app's container image already has them.
          </span>
        </div>
      )}
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={() => setDrag(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDrag(false);
          void send([...e.dataTransfer.files]);
        }}
        className={cn(
          'flex flex-col items-center gap-3 rounded-xl border-2 border-dashed px-6 py-10 text-center transition-colors duration-[var(--dur-fast)]',
          drag ? 'border-primary bg-primary/5' : 'border-border',
        )}
      >
        {busy ? <Loader2 className="size-8 animate-spin text-muted-foreground" /> : <FileUp className="size-8 text-muted-foreground" />}
        <div>
          <div className="font-medium">Drop credit-card statement PDFs here</div>
          <div className="text-sm text-muted-foreground">HSBC and Citibank Singapore are supported fully; other banks' “date · description · amount” layouts usually work.</div>
        </div>
        <Button variant="outline" onClick={() => input.current?.click()} disabled={busy}>
          Choose files
        </Button>
        <input
          ref={input}
          type="file"
          accept="application/pdf,.pdf"
          multiple
          hidden
          onChange={(e) => {
            void send([...(e.target.files ?? [])]);
            e.target.value = '';
          }}
        />
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <ShieldCheck className="size-3.5" /> Read on this computer — the PDF's own text, or OCR for image-only statements. Never sent anywhere.
        </div>
      </div>
      <ErrorNote error={err} />
      {notes.map((n) => (
        <div key={n} className="rounded-md bg-muted px-3 py-2 text-sm">
          {n}
        </div>
      ))}

      {statements.length === 0 ? (
        <Empty icon={FileText} title="No statements yet">
          Import a few months to see trends. Each statement is checked against its own totals, so you know every line was read.
        </Empty>
      ) : (
        <Section title="Imported">
          <ul className="divide-y">
            {statements.map((s) => (
              <StatementRow key={s.id} s={s} />
            ))}
          </ul>
        </Section>
      )}
    </div>
  );
}

function statusLine(s: Statement) {
  switch (s.status) {
    case 'queued':
      return { icon: Loader2, spin: true, cls: 'text-muted-foreground', text: 'Waiting…' };
    case 'reading':
      return { icon: Loader2, spin: true, cls: 'text-muted-foreground', text: s.progress || 'Reading…' };
    case 'done':
      return s.recon.checked
        ? { icon: CheckCircle2, cls: 'text-[var(--good)]', text: 'Every line matches the statement totals' }
        : { icon: CheckCircle2, cls: 'text-muted-foreground', text: 'Read (no totals to check against)' };
    case 'review':
      return { icon: TriangleAlert, cls: 'text-[var(--warn)]', text: gapText(s) };
    case 'failed':
      return { icon: XCircle, cls: 'text-[var(--bad)]', text: s.error || 'Failed' };
  }
}

function gapText(s: Statement): string {
  const parts: string[] = [];
  if (s.recon.debitsGap) parts.push(`purchases off by ${money(s.recon.debitsGap)}`);
  if (s.recon.creditsGap) parts.push(`credits off by ${money(s.recon.creditsGap)}`);
  return `Needs a look: ${parts.join(', ')}`;
}

function StatementRow({ s }: { s: Statement }) {
  const st = statusLine(s);
  const Icon = st.icon;
  return (
    <li>
      <a href={href('import', s.id)} className="flex items-center gap-3 px-1 py-3 hover:bg-accent/50">
        <FileText className="size-5 shrink-0 text-muted-foreground" />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-sm font-medium">
            {s.summary.product ? `${s.summary.product}${s.summary.last4 ? ` ··${s.summary.last4}` : ''}` : s.fileName}
            {s.periodStart && (
              <span className="font-normal text-muted-foreground">
                {' '}
                · {date(s.periodStart)} – {date(s.periodEnd, true)}
              </span>
            )}
          </span>
          <span className={cn('flex items-center gap-1.5 text-xs', st.cls)}>
            <Icon className={cn('size-3.5', st.spin && 'animate-spin')} /> {st.text}
          </span>
        </span>
        {s.txns > 0 && <span className="text-xs text-muted-foreground">{plural(s.txns, 'line')}</span>}
      </a>
    </li>
  );
}

function StatementDetail({ id }: { id: number }) {
  const { data, error } = useStatement(id);
  const inv = useInvalidateLedger();
  const [edit, setEdit] = useState<Txn | null>(null);
  const [page, setPage] = useState<number | null>(null);
  if (error) return <ErrorNote error={error} />;
  if (!data) return <Loader2 className="m-8 size-5 animate-spin text-muted-foreground" />;
  const s = data.statement;
  const txns = data.transactions;
  const st = statusLine(s);
  const Icon = st.icon;
  const pages = Array.from({ length: Math.max(s.pages, 1) }, (_, i) => i + 1);
  const rows: [string, number | null | undefined, number][] = [
    ['Purchases & debits', s.summary.debits, s.recon.debits ?? 0],
    ['Payments & credits', s.summary.credits, s.recon.credits ?? 0],
  ];

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="ghost" size="icon-sm" onClick={() => go('import')} aria-label="Back">
          <ArrowLeft className="size-4" />
        </Button>
        <h1 className="text-xl font-semibold tracking-tight">{s.summary.product || s.fileName}</h1>
        {s.periodStart && (
          <span className="text-sm text-muted-foreground">
            {date(s.periodStart)} – {date(s.periodEnd, true)}
          </span>
        )}
        <div className="ml-auto flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={async () => {
              if (window.confirm('Read this statement again? Edits and splits on its rows are replaced (your rules are kept).')) {
                await api.reread(id);
                inv();
              }
            }}
          >
            <RefreshCw className="size-4" /> Read again
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive"
            onClick={async () => {
              if (window.confirm('Remove this statement and its transactions?')) {
                await api.deleteStatement(id);
                inv();
                go('import');
              }
            }}
          >
            <Trash2 className="size-4" /> Remove
          </Button>
        </div>
      </div>

      <div className={cn('flex items-center gap-2 text-sm', st.cls)}>
        <Icon className={cn('size-4', st.spin && 'animate-spin')} /> {st.text}
      </div>

      {(s.status === 'done' || s.status === 'review') && (
        <Section title="Checked against the statement">
          <table className="w-full text-sm">
            <thead className="text-xs text-muted-foreground">
              <tr>
                <th className="py-1 text-left font-medium" />
                <th className="py-1 text-right font-medium">Statement says</th>
                <th className="py-1 text-right font-medium">Lines add up to</th>
                <th className="w-8" />
              </tr>
            </thead>
            <tbody>
              {rows.map(([label, said, got]) => (
                <tr key={label} className="border-t">
                  <td className="py-1.5">{label}</td>
                  <td className="tabular py-1.5 text-right">{said == null ? '—' : money(said)}</td>
                  <td className="tabular py-1.5 text-right">{money(got)}</td>
                  <td className="text-right">
                    {said == null ? null : said === got ? (
                      <CheckCircle2 className="ml-auto size-4 text-[var(--good)]" aria-label="matches" />
                    ) : (
                      <TriangleAlert className="ml-auto size-4 text-[var(--warn)]" aria-label="differs" />
                    )}
                  </td>
                </tr>
              ))}
              {s.summary.total != null && (
                <tr className="border-t">
                  <td className="py-1.5">Total balance</td>
                  <td className="tabular py-1.5 text-right">{money(s.summary.total)}</td>
                  <td className="py-1.5 text-right text-xs text-muted-foreground">
                    {s.recon.summaryOk ? 'previous + purchases − credits ✓' : 'summary doesn’t add up'}
                  </td>
                  <td />
                </tr>
              )}
            </tbody>
          </table>
          {s.warnings.length > 0 && (
            <ul className="mt-3 flex flex-col gap-1 text-sm text-[var(--warn)]">
              {s.warnings.map((w) => (
                <li key={w}>{w}</li>
              ))}
            </ul>
          )}
          <p className="mt-3 text-xs text-muted-foreground">
            Read with {plural(s.passes.length, 'OCR pass', 'OCR passes')}
            {s.passes.length > 1 ? ' (later passes re-read the page at higher resolution to fill gaps)' : ''}. Missing a line? Add it in Transactions → Add.
          </p>
        </Section>
      )}

      <div className="grid gap-5 lg:grid-cols-[1fr_1.1fr]">
        <Section title={`Lines (${txns.length})`}>
          <TxnList txns={txns} onPick={setEdit} showCategory />
          <p className="mt-2 text-xs text-muted-foreground">
            Total in: <Money cents={txns.filter((t) => t.amount < 0).reduce((a, t) => a + t.amount, 0)} /> · out:{' '}
            <Money cents={txns.filter((t) => t.amount > 0).reduce((a, t) => a + t.amount, 0)} />
          </p>
        </Section>
        <Section title="Pages">
          <div className="mb-2 flex flex-wrap gap-1">
            {pages.map((p) => (
              <Button key={p} size="sm" variant={(page ?? 1) === p ? 'secondary' : 'ghost'} onClick={() => setPage(p)}>
                {p}
              </Button>
            ))}
          </div>
          <img
            key={page ?? 1}
            src={`/api/statements/${id}/pages/${page ?? 1}.png`}
            alt={`Statement page ${page ?? 1}`}
            className="w-full rounded-md border bg-white"
            loading="lazy"
          />
        </Section>
      </div>
      <TxnSheet txn={edit} onClose={() => setEdit(null)} />
    </div>
  );
}
