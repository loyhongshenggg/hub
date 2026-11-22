// Typed client for the Finance Go API (apps/finance/api). Money is integer
// cents; transaction amounts are + money out, − money in.

export type Kind =
  | 'purchase'
  | 'refund'
  | 'fee'
  | 'payment'
  | 'income'
  | 'transfer';

export interface Category {
  id: string;
  name: string;
  group: string;
  essential: boolean;
}

export interface Group {
  id: string;
  name: string;
  slot: number; // 1..6 categorical colour slot; 0 = neutral
}

export interface Catalog {
  categories: Category[];
  groups: Group[];
  kinds: Kind[];
  goalKinds: GoalKind[];
  areas: string[];
}

export interface Account {
  id: number;
  name: string;
  institution: string;
  kind: string;
  last4: string;
  currency: string;
}

export interface DirectorySource {
  source: 'osm' | 'acra';
  count: number;
  refreshedAt: string | null;
  status: '' | 'running' | 'ok' | 'failed';
  error?: string;
}

export interface Usage {
  month: string;
  calls: number;
  inputTokens: number;
  outputTokens: number;
  searches: number;
  costMicros: number;
}

export interface Status {
  ocr: { ok: boolean; error: string };
  claude: {
    keySource: '' | 'settings' | 'env';
    goals: boolean;
    merchants: boolean;
    capCents: number;
    usage: Usage;
  };
  directory: DirectorySource[];
  review: number;
  dataRange: { first: string; last: string };
  accounts: Account[];
  owedToYou: number;
  today: string;
}

export interface Txn {
  id: number;
  accountId: number | null;
  statementId: number | null;
  date: string;
  posted: string;
  raw: string;
  amount: number;
  kind: Kind;
  category: string;
  merchant: string;
  merchantKey: string;
  matchSource: string;
  confidence: number;
  detail?: string;
  needsReview: boolean;
  foreign: boolean;
  excluded: boolean;
  oneOff: boolean;
  note: string;
  source: 'statement' | 'manual';
  page: number;
  y: number;
  recovered: boolean;
  splitMethod?: SplitMethod | '';
  myWeight?: number;
  othersShare: number;
  myCost: number | null; // your cost typed by hand
  myAmount: number;
}

export type SplitMethod = 'equal' | 'shares' | 'exact';

export interface Share {
  id: number;
  txnId: number;
  personId: number;
  name: string;
  amount: number;
  weight: number;
  settledAt: string | null;
  date?: string;
  merchant?: string;
  total?: number;
}

export interface Person {
  id: number;
  name: string;
  owed: number;
  settled: number;
  count: number;
  last: string | null;
}

export interface SplitPart {
  name: string;
  weight?: number;
  amount?: number;
}

export interface CategoryStat {
  id: string;
  name: string;
  group: string;
  amount: number;
  count: number;
  prev: number;
  avg: number;
  share: number;
}

export interface MerchantStat {
  name: string;
  key: string;
  category: string;
  amount: number;
  count: number;
}

export interface MonthStat {
  month: string;
  total: number;
  byGroup: Record<string, number>;
  hasData: boolean;
}

export interface DayPoint {
  day: number;
  value: number;
}

export interface Recurring {
  name: string;
  key: string;
  category: string;
  amount: number;
  every: 'month' | 'year';
  last: string;
  monthly: number;
}

export interface Insight {
  id: string;
  tone: 'good' | 'info' | 'warn';
  title: string;
  body: string;
  category?: string;
  merchant?: string;
  txnId?: number;
}

export interface Overview {
  month: string;
  months: string[];
  partial: boolean;
  dataEnd: string;
  spent: number;
  count: number;
  prevSpent: number;
  avgSpent: number;
  avgMonths: number;
  refunds: number;
  income: number;
  categories: CategoryStat[];
  merchants: MerchantStat[];
  trend: MonthStat[];
  pace: DayPoint[];
  avgPace: DayPoint[] | null;
  recurring: Recurring[];
  recurringPerMonth: number;
  insights: Insight[];
}

export interface Summary {
  bank: string;
  product: string;
  last4: string;
  currency: string;
  periodStart: string;
  periodEnd: string;
  dueDate: string;
  prevBalance: number | null;
  credits: number | null;
  debits: number | null;
  total: number | null;
}

export interface Recon {
  checked: boolean;
  ok: boolean;
  debits: number;
  credits: number;
  debitsGap: number;
  creditsGap: number;
  summaryOk: boolean;
}

export type StatementStatus = 'queued' | 'reading' | 'done' | 'review' | 'failed';

export interface Statement {
  id: number;
  accountId: number | null;
  fileName: string;
  status: StatementStatus;
  progress: string;
  error?: string;
  periodStart: string;
  periodEnd: string;
  summary: Partial<Summary>;
  recon: Partial<Recon>;
  warnings: string[];
  passes: string[];
  pages: number;
  importedAt: string;
  finishedAt: string | null;
  txns: number;
}

export type GoalKind = 'target' | 'payoff' | 'limit' | 'habit';
export type GoalStatus = 'active' | 'paused' | 'achieved' | 'archived';
export type Period = 'week' | 'month' | 'year';

export interface Milestone {
  label: string;
  amount?: number;
  date?: string;
  doneAt?: string;
}

export interface GoalFilter {
  categories: string[] | null;
  merchants: string[] | null;
  keywords: string[] | null;
}

export interface Goal {
  id: number;
  kind: GoalKind;
  title: string;
  why: string;
  area: string;
  status: GoalStatus;
  target: number;
  startAmount: number;
  startDate: string;
  targetDate: string;
  period: Period;
  metric: 'amount' | 'count';
  filter: GoalFilter;
  monthlyPlan: number;
  milestones: Milestone[];
  source: 'manual' | 'claude';
  prompt?: string;
  sort: number;
  createdAt?: string;
  updatedAt?: string;
  achievedAt: string | null;
}

export type ProgressStatus =
  | 'achieved'
  | 'ahead'
  | 'on_track'
  | 'at_risk'
  | 'behind'
  | 'over'
  | 'no_data'
  | 'paused';

export interface GoalPoint {
  date: string;
  label: string;
  value: number;
  met?: boolean;
}

export interface Progress {
  current: number;
  percent: number;
  remaining: number;
  status: ProgressStatus;
  statusText: string;
  neededPerMonth?: number;
  projectedDate?: string;
  projected?: number;
  periodLabel?: string;
  periodStart?: string;
  periodEnd?: string;
  history: GoalPoint[];
  streak: number;
  milestones: (Milestone & { reached: boolean })[];
  lastCheckIn?: string;
}

export interface GoalEntry {
  id: number;
  goalId: number;
  date: string;
  amount: number;
  note: string;
}

export interface GoalView extends Goal {
  progress: Progress;
  entries?: GoalEntry[];
}

export interface Drafts {
  drafts: Goal[];
  reasoning: string[];
  assumptions: string[];
  questions: string[];
  sharedAverages: boolean;
  engine: 'local' | 'claude';
}

export interface ClaudeSettings {
  keySet: boolean;
  keySource: '' | 'settings' | 'env';
  keyHint: string;
  goals: boolean;
  merchants: boolean;
  shareAverages: boolean;
  capCents: number;
  usage: Usage;
  model: string;
}

export interface Rule {
  key: string;
  merchant: string;
  category: string;
  kind?: string;
  createdAt: string;
}

export interface TxnQuery {
  month?: string;
  from?: string;
  to?: string;
  category?: string;
  kind?: string;
  q?: string;
  review?: boolean;
  merchant?: string;
  merchantKey?: string;
  statement?: number;
  spending?: boolean;
  split?: boolean;
  limit?: number;
}

export interface TxnPatch {
  date?: string;
  amount?: number;
  kind?: Kind;
  category?: string;
  merchant?: string;
  note?: string;
  excluded?: boolean;
  oneOff?: boolean;
  reviewed?: boolean;
  applyToSimilar?: boolean;
  myCost?: number; // positive cents
  clearMyCost?: boolean;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {} };
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)['Content-Type'] = 'application/json';
  }
  const r = await fetch(path, init);
  if (r.status === 204) return undefined as T;
  const text = await r.text();
  const data = text ? (JSON.parse(text) as unknown) : undefined;
  if (!r.ok) {
    const msg =
      (data as { error?: string } | undefined)?.error ?? `${r.status} ${r.statusText}`;
    throw new ApiError(r.status, msg);
  }
  return data as T;
}

function qs(q: object): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === '' || v === false || v === null) continue;
    p.set(k, v === true ? '1' : String(v));
  }
  const s = p.toString();
  return s ? `?${s}` : '';
}

/**
 * goalBody drops what the server computes (a GoalView's progress and
 * entries): callers often pass a whole view, and the API rejects unknown
 * fields.
 */
export function goalBody(g: Partial<Goal> & Partial<Pick<GoalView, 'progress' | 'entries'>>): Partial<Goal> {
  const { progress: _progress, entries: _entries, ...goal } = g;
  return goal;
}

export const api = {
  status: () => req<Status>('GET', '/api/status'),
  catalog: () => req<Catalog>('GET', '/api/catalog'),
  overview: (month?: string, spread?: boolean) =>
    req<Overview>('GET', `/api/overview${qs({ month, spread })}`),
  txns: (q: TxnQuery) =>
    req<{ transactions: Txn[] }>('GET', `/api/transactions${qs(q)}`).then(
      (r) => r.transactions,
    ),
  addTxn: (t: {
    date: string;
    amount: number;
    kind: Kind;
    category: string;
    merchant: string;
    note: string;
  }) => req<Txn>('POST', '/api/transactions', t),
  patchTxn: (id: number, p: TxnPatch) =>
    req<{ transaction: Txn; applied: number }>('PATCH', `/api/transactions/${id}`, p),
  deleteTxn: (id: number) => req<void>('DELETE', `/api/transactions/${id}`),
  rules: () => req<{ rules: Rule[] }>('GET', '/api/rules').then((r) => r.rules),
  split: (id: number) => req<{ transaction: Txn; shares: Share[] }>('GET', `/api/transactions/${id}/split`),
  setSplit: (id: number, body: { method: SplitMethod; myWeight: number; parts: SplitPart[] }) =>
    req<{ transaction: Txn; shares: Share[] }>('PUT', `/api/transactions/${id}/split`, body),
  clearSplit: (id: number) => req<{ transaction: Txn; shares: Share[] }>('DELETE', `/api/transactions/${id}/split`),
  people: () => req<{ people: Person[]; owedToYou: number }>('GET', '/api/people'),
  personShares: (id: number) => req<{ shares: Share[] }>('GET', `/api/people/${id}/shares`).then((r) => r.shares),
  settleShare: (id: number, settled: boolean) => req<void>('POST', `/api/shares/${id}/settle`, { settled }),
  settlePerson: (id: number) => req<{ settled: number }>('POST', `/api/people/${id}/settle`),
  renamePerson: (id: number, name: string) => req<void>('PATCH', `/api/people/${id}`, { name }),
  deleteRule: (key: string) =>
    req<void>('DELETE', `/api/rules/${encodeURIComponent(key)}`),

  statements: () =>
    req<{ statements: Statement[] }>('GET', '/api/statements').then((r) => r.statements),
  statement: (id: number) =>
    req<{ statement: Statement; transactions: Txn[] }>('GET', `/api/statements/${id}`),
  upload: (files: File[]) => {
    const fd = new FormData();
    for (const f of files) fd.append('files', f);
    return req<{
      uploads: { id: number; fileName: string; duplicate: boolean; error?: string }[];
    }>('POST', '/api/statements', fd);
  },
  deleteStatement: (id: number) => req<void>('DELETE', `/api/statements/${id}`),
  reread: (id: number) => req<void>('POST', `/api/statements/${id}/reread`),

  goals: () => req<{ goals: GoalView[] }>('GET', '/api/goals').then((r) => r.goals),
  reorderGoals: (ids: number[]) => req<void>('PUT', '/api/goals/order', { ids }),
  goal: (id: number) => req<GoalView>('GET', `/api/goals/${id}`),
  createGoal: (g: Partial<Goal>) => req<GoalView>('POST', '/api/goals', goalBody(g)),
  updateGoal: (g: Goal) => req<GoalView>('PUT', `/api/goals/${g.id}`, goalBody(g)),
  deleteGoal: (id: number) => req<void>('DELETE', `/api/goals/${id}`),
  addEntry: (id: number, e: { date: string; amount: number; note: string }) =>
    req<GoalView>('POST', `/api/goals/${id}/entries`, e),
  deleteEntry: (id: number, entry: number) =>
    req<GoalView>('DELETE', `/api/goals/${id}/entries/${entry}`),
  draftGoals: (paragraph: string, shareAverages: boolean) =>
    req<Drafts>('POST', '/api/goals/draft', { paragraph, shareAverages }),

  claude: () => req<ClaudeSettings>('GET', '/api/settings/claude'),
  saveClaude: (s: Partial<ClaudeSettings> & { apiKey?: string }) => {
    const { apiKey, goals, merchants, shareAverages, capCents } = s;
    return req<ClaudeSettings>('PUT', '/api/settings/claude', {
      apiKey,
      goals,
      merchants,
      shareAverages,
      capCents,
    });
  },
  refreshDirectory: (source: string) =>
    req<void>('POST', `/api/directory/${source}/refresh`),
  lookupMerchants: () => req<{ updated: number }>('POST', '/api/merchants/lookup'),
  renameAccount: (id: number, name: string) =>
    req<void>('PATCH', `/api/accounts/${id}`, { name }),
};
