// Formatting for money (SGD cents) and dates.

const nf = new Intl.NumberFormat('en-SG', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});
const nf0 = new Intl.NumberFormat('en-SG', { maximumFractionDigits: 0 });

/** money: S$1,234.56 (whole dollars from S$10,000, or when `whole`). */
export function money(cents: number, opts: { whole?: boolean; sign?: boolean } = {}): string {
  const neg = cents < 0;
  const v = Math.abs(cents) / 100;
  const s = opts.whole || v >= 10000 ? nf0.format(Math.round(v)) : nf.format(v);
  const sign = neg ? '−' : opts.sign ? '+' : '';
  return `${sign}S$${s}`;
}

/** compact: S$1.2k for chart axes. */
export function compact(cents: number): string {
  const v = cents / 100;
  if (Math.abs(v) >= 1000) return `S$${(v / 1000).toFixed(v >= 10000 ? 0 : 1)}k`;
  return `S$${Math.round(v)}`;
}

/** dollars ↔ cents for inputs. */
/**
 * toCents reads an amount someone typed — "1700", "1,700.50", "S$ 1 700",
 * "80k", "1.2m" — into cents. Anything else is NaN (never a silent guess).
 */
export function toCents(s: string): number {
  const m = /^\s*(-)?\s*(?:S?\$)?\s*(\d(?:[\d, ]*\d)?(?:\.\d*)?|\.\d+)\s*([kKmM])?\s*$/.exec(s);
  if (!m) return NaN;
  const scale = m[3] ? (m[3].toLowerCase() === 'k' ? 1e3 : 1e6) : 1;
  const n = Math.round(Number(m[2].replace(/[, ]/g, '')) * scale * 100);
  return m[1] ? -n : n;
}
export const toDollars = (c: number): string => (c ? String(c / 100) : '');

function parseDate(d: string): Date {
  const [y, m, day] = d.split('-').map(Number);
  return new Date(y, (m ?? 1) - 1, day ?? 1);
}

export function date(d: string, withYear = false): string {
  if (!d) return '';
  return parseDate(d).toLocaleDateString('en-SG', {
    day: 'numeric',
    month: 'short',
    ...(withYear ? { year: 'numeric' } : {}),
  });
}

export function monthName(m: string, short = false): string {
  if (!m) return '';
  return parseDate(`${m}-01`).toLocaleDateString('en-SG', {
    month: short ? 'short' : 'long',
    year: short ? '2-digit' : 'numeric',
  });
}

export function monthShort(m: string): string {
  return parseDate(`${m}-01`).toLocaleDateString('en-SG', { month: 'short' });
}

export function monthYear(d: string): string {
  if (!d) return '';
  return parseDate(d).toLocaleDateString('en-SG', { month: 'short', year: 'numeric' });
}

export function today(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export function pct(x: number): string {
  return `${Math.round(x * 100)}%`;
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`;
}

export function relTime(iso: string | null | undefined): string {
  if (!iso) return 'never';
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 90) return 'just now';
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return `${Math.round(s / 86400)} days ago`;
}

export const areaNames: Record<string, string> = {
  safety: 'Safety net',
  home: 'Home',
  retirement: 'Retirement',
  investing: 'Investing',
  debt: 'Debt',
  lifestyle: 'Lifestyle',
  education: 'Education',
  other: 'Other',
};

export const kindNames: Record<string, string> = {
  target: 'Save up',
  payoff: 'Pay off',
  limit: 'Spending limit',
  habit: 'Regular habit',
};

export const txnKindNames: Record<string, string> = {
  purchase: 'Purchase',
  refund: 'Refund',
  fee: 'Fee',
  payment: 'Card payment',
  income: 'Income',
  transfer: 'Transfer',
};
