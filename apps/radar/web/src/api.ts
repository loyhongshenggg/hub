// Typed client for the Radar Go API (apps/radar/api). Learning types mirror
// the JSON of apps/radar/api/internal/engine and internal/reader.

export type CategoryId = 'swe' | 'security' | 'ai';

export interface Item {
  id: number;
  title: string;
  url: string;
  summary: string;
  publishedAt: string;
  read: boolean;
  saved: boolean;
  topics: string[];
  difficulty?: number;
  type?: StepType;
  minutes?: number;
  taggedBy?: 'rules' | 'claude' | 'user';
  source: { id: string; name: string; category: CategoryId; mentor?: string };
}

export interface Counts {
  unread: number;
  unreadByCategory: Partial<Record<CategoryId, number>>;
  saved: number;
  newSinceVisit: number;
}

export interface FetchStatus {
  running: boolean;
  lastRound: string | null;
}

export interface Source {
  id: string;
  name: string;
  url: string;
  category: CategoryId;
  active: boolean;
  lastFetchAt: string | null;
  lastOkAt: string | null;
  lastError?: string;
  items: number;
}

export interface NewsFilter {
  category: CategoryId | 'all';
  q: string;
  unread: boolean;
  saved: boolean;
  tagged?: boolean;
  limit?: number;
}

// --- learning (apps/radar/api/internal/engine) ---

export type StepType =
  | 'read'
  | 'watch'
  | 'lab'
  | 'tool'
  | 'walkthrough'
  | 'checkpoint';
export type Rating = '' | 'easy' | 'right' | 'hard';

export interface StepView {
  ref: string;
  kind:
    | 'step'
    | 'corrective'
    | 'fresh'
    | 'practice'
    | 'checkpoint'
    | 'capstone';
  title: string;
  type: StepType;
  url?: string;
  by?: string;
  minutes: number;
  focus?: string;
  reason?: string;
  steps?: string[];
  status?: 'done' | 'skipped' | 'dismissed';
  rating?: Rating;
  optional?: boolean;
}

export interface ChapterView {
  track: string;
  trackTitle: string;
  chapter: string;
  index: number;
  count: number;
  title: string;
  summary: string;
  difficulty: number;
  topics: string[];
  outcomes: string[];
  steps: StepView[];
  practice?: StepView;
  checkpoint: StepView;
  met: boolean[];
  started: boolean;
  completed: boolean;
  offerSkip: boolean;
  next: StepView | null;
  minutes: number;
  minutesDone: number;
  stepsDone: number;
  stepsTotal: number;
}

export interface Recommendation {
  kind: 'chapter' | 'capstone';
  track: string;
  trackTitle: string;
  chapter?: string;
  chapterTitle?: string;
  index: number;
  count: number;
  difficulty?: number;
  resume: boolean;
  next: StepView | null;
  stepsDone: number;
  stepsTotal: number;
  minutesLeft: number;
  why: string[];
}

export interface TrackSummary {
  id: string;
  title: string;
  tagline: string;
  area: string;
  level: string;
  mentors: string[];
  outline: boolean;
  chapters: number;
  completed: number;
  minutes: number;
  minutesDone: number;
  status: 'new' | 'active' | 'done';
}

export interface Week {
  start: string;
  goal: number;
  minutes: number;
  days: number[];
}

export interface Dashboard {
  needsAssessment: boolean;
  continue: Recommendation | null;
  upNext: Recommendation[];
  week: Week;
  tracks: TrackSummary[];
  fresh: number;
}

export interface ChapterSummary {
  id: string;
  title: string;
  summary: string;
  difficulty: number;
  topics: string[];
  minutes: number;
  status: 'new' | 'active' | 'done';
  stepsDone: number;
  stepsTotal: number;
}

export interface Mentor {
  id: string;
  name: string;
  handle?: string;
  role: string;
  focus: string;
  area: string;
  links: { label: string; url: string }[];
}

export interface TrackView extends TrackSummary {
  chapterList: ChapterSummary[];
  capstone?: {
    title: string;
    summary: string;
    outcomes: string[];
    steps: string[];
  };
  capstoneDone: boolean;
  mentorInfo: Mentor[];
}

export interface HistoryEntry {
  ref: string;
  status: string;
  at: string;
  minutes: number;
  rating?: Rating;
  worth?: boolean;
  kind: 'step' | 'practice' | 'capstone' | 'item';
  title: string;
  url?: string;
  by?: string;
  track?: string;
  trackTitle?: string;
  chapter?: string;
  chapterTitle?: string;
}

export interface Topic {
  id: string;
  name: string;
  area: string;
  requires: string[] | null;
}

export interface Curriculum {
  areas: { id: string; name: string }[];
  topics: Topic[];
  mentors: Mentor[];
  tracks: { id: string; title: string; status?: string }[];
}

export interface Settings {
  weeklyMinutes: number;
  priorities: Record<string, number>;
  timezone: string;
}

export type Level = 'new' | 'some' | 'solid';

export interface Assessment {
  done: boolean;
  topics: { id: string; name: string; area: string; level?: Level }[];
}

export interface ReaderView {
  mode: 'reader' | 'video' | 'live' | 'outside';
  url: string;
  embed?: string;
  note?: string;
  article?: {
    title: string;
    byline?: string;
    site?: string;
    published?: string;
    image?: string;
    html: string;
    words: number;
    minutes: number;
  };
}

export interface CheckpointResult {
  completed: boolean;
  corrective?: string;
}

async function call<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      msg = ((await res.json()) as { error?: string }).error ?? msg;
    } catch {
      // not JSON
    }
    throw new Error(msg);
  }
  return (
    res.status === 204 || res.status === 202 ? undefined : await res.json()
  ) as T;
}

function query(f: NewsFilter, cursor?: string): string {
  const p = new URLSearchParams({
    category: f.category,
    limit: String(f.limit ?? 40),
  });
  if (f.tagged) p.set('tagged', '1');
  if (f.q.trim()) p.set('q', f.q.trim());
  if (f.unread) p.set('unread', '1');
  if (f.saved) p.set('saved', '1');
  if (cursor) p.set('cursor', cursor);
  return p.toString();
}

export const api = {
  news: (f: NewsFilter, cursor?: string) =>
    call<{ items: Item[]; nextCursor: string }>(
      'GET',
      `/api/news?${query(f, cursor)}`,
    ),
  setRead: (id: number, read: boolean) =>
    call<void>('POST', `/api/items/${id}/read`, { read }),
  setSaved: (id: number, saved: boolean) =>
    call<void>('POST', `/api/items/${id}/saved`, { saved }),
  readAll: (f: NewsFilter) =>
    call<{ marked: number }>('POST', `/api/news/read-all?${query(f)}`),
  summary: () =>
    call<{ counts: Counts; fetch: FetchStatus }>('GET', '/api/summary'),
  sources: () =>
    call<{ sources: Source[]; fetch: FetchStatus }>('GET', '/api/sources'),
  refresh: () => call<void>('POST', '/api/refresh'),
  visit: () => call<void>('POST', '/api/visit'),

  curriculum: () => call<Curriculum>('GET', '/api/curriculum'),
  dashboard: () => call<Dashboard>('GET', '/api/learning'),
  track: (id: string) =>
    call<TrackView>('GET', `/api/learning/tracks/${enc(id)}`),
  chapter: (track: string, chapter: string) =>
    call<ChapterView>('GET', chapterPath(track, chapter)),
  done: (ref: string, rating: Rating, worth: boolean | null) =>
    call<{ chapter: ChapterView | null }>('POST', '/api/learning/done', {
      ref,
      rating,
      worth: worth ?? undefined,
    }),
  undo: (ref: string) =>
    call<{ chapter: ChapterView | null }>('POST', '/api/learning/undo', {
      ref,
    }),
  dismiss: (ref: string) =>
    call<{ chapter: ChapterView | null }>('POST', '/api/learning/dismiss', {
      ref,
    }),
  skipToPractice: (track: string, chapter: string) =>
    call<ChapterView>(
      'POST',
      `${chapterPath(track, chapter)}/skip-to-practice`,
    ),
  checkpoint: (
    track: string,
    chapter: string,
    body: { met: boolean[]; rating: Rating; worth?: boolean; force: boolean },
  ) =>
    call<{ result: CheckpointResult; chapter: ChapterView }>(
      'POST',
      `${chapterPath(track, chapter)}/checkpoint`,
      body,
    ),
  snooze: (track: string, chapter: string) =>
    call<void>('POST', `${chapterPath(track, chapter)}/snooze`),
  history: (before?: string) =>
    call<{ items: HistoryEntry[]; nextBefore: string }>(
      'GET',
      `/api/learning/history?limit=40${before ? `&before=${enc(before)}` : ''}`,
    ),
  assessment: () => call<Assessment>('GET', '/api/learning/assessment'),
  assess: (answers: Record<string, Level>) =>
    call<void>('PUT', '/api/learning/assessment', { answers }),
  settings: () => call<Settings>('GET', '/api/learning/settings'),
  saveSettings: (s: Settings) =>
    call<Settings>('PUT', '/api/learning/settings', s),
  fresh: (limit = 6) =>
    call<{ items: Item[]; nextCursor: string }>(
      'GET',
      `/api/learning/fresh?limit=${limit}`,
    ),
  reader: (ref: string) =>
    call<ReaderView>('GET', `/api/reader?ref=${enc(ref)}`),
};

function enc(s: string): string {
  return encodeURIComponent(s);
}

function chapterPath(track: string, chapter: string): string {
  return `/api/learning/tracks/${enc(track)}/chapters/${enc(chapter)}`;
}
