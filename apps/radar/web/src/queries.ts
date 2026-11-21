// Server state for the learning UI (TanStack Query). Mutations update the
// cached chapter optimistically so Done feels instant; the server's answer
// then replaces the guess.
import {
  QueryClient,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import {
  api,
  type ChapterView,
  type Level,
  type Rating,
  type Settings,
  type StepView,
} from './api';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, refetchOnWindowFocus: true, retry: 1 },
  },
});

export const keys = {
  curriculum: ['curriculum'] as const,
  dashboard: ['learning'] as const,
  track: (id: string) => ['track', id] as const,
  chapter: (t: string, c: string) => ['chapter', t, c] as const,
  reader: (ref: string) => ['reader', ref] as const,
  history: ['history'] as const,
  settings: ['settings'] as const,
  assessment: ['assessment'] as const,
  newsPanel: ['news-panel'] as const,
};

export const useCurriculum = () =>
  useQuery({
    queryKey: keys.curriculum,
    queryFn: api.curriculum,
    staleTime: Infinity,
  });

export const useDashboard = () =>
  useQuery({ queryKey: keys.dashboard, queryFn: api.dashboard });

export const useTrack = (id: string) =>
  useQuery({ queryKey: keys.track(id), queryFn: () => api.track(id) });

export const useChapter = (t: string, c: string) =>
  useQuery({
    queryKey: keys.chapter(t, c),
    queryFn: () => api.chapter(t, c),
  });

const readerQuery = (ref: string) => ({
  queryKey: keys.reader(ref),
  queryFn: () => api.reader(ref),
  staleTime: 10 * 60_000,
  gcTime: 30 * 60_000,
});

export const useReader = (ref: string | null) =>
  useQuery({ ...readerQuery(ref ?? ''), enabled: !!ref });

/** Warm the reader for a step the learner is likely to open next. */
export function prefetchReader(qc: QueryClient, ref: string | undefined) {
  if (ref && !ref.startsWith('checkpoint:') && !ref.startsWith('capstone:'))
    void qc.prefetchQuery(readerQuery(ref));
}

export const useHistory = () =>
  useInfiniteQuery({
    queryKey: keys.history,
    queryFn: ({ pageParam }) => api.history(pageParam || undefined),
    initialPageParam: '',
    getNextPageParam: (last) => last.nextBefore || undefined,
  });

export const useSettings = () =>
  useQuery({ queryKey: keys.settings, queryFn: api.settings });

export const useAssessment = () =>
  useQuery({ queryKey: keys.assessment, queryFn: api.assessment });

export const useFresh = () =>
  useQuery({ queryKey: ['fresh'], queryFn: () => api.fresh(4) });

export const useNewsPanel = () =>
  useQuery({
    queryKey: keys.newsPanel,
    queryFn: () =>
      api.news({
        category: 'all',
        q: '',
        unread: false,
        saved: false,
        tagged: true,
        limit: 10,
      }),
    refetchInterval: 5 * 60_000,
  });

// --- local view of progress ------------------------------------------------

/** The first required step not done or skipped, else practice, else the checkpoint. */
export function nextOf(v: ChapterView): StepView | null {
  if (v.completed) return null;
  const step = v.steps.find((s) => !s.optional && !s.status);
  if (step) return step;
  if (v.practice && !v.practice.status) return v.practice;
  return v.checkpoint;
}

/** All rows of a chapter in reading order: steps, practice, checkpoint. */
export function rowsOf(v: ChapterView): StepView[] {
  return [...v.steps, ...(v.practice ? [v.practice] : []), v.checkpoint];
}

/** Where to go after finishing ref: the next unfinished row after it, else the chapter's next. */
export function afterDone(v: ChapterView, ref: string): string | null {
  const rows = rowsOf(v);
  const i = rows.findIndex((r) => r.ref === ref);
  const later = rows
    .slice(i + 1)
    .find((r) => !r.status && (!r.optional || r.kind === 'checkpoint'));
  if (later) return later.ref;
  return nextOf(markDone(v, ref, ''))?.ref ?? null;
}

function markDone(v: ChapterView, ref: string, rating: Rating): ChapterView {
  const tick = (s: StepView): StepView =>
    s.ref === ref ? { ...s, status: 'done', rating } : s;
  const next: ChapterView = {
    ...v,
    steps: v.steps.map(tick),
    practice: v.practice && tick(v.practice),
    started: true,
  };
  next.stepsDone = next.steps.filter((s) => !s.optional && s.status).length;
  next.next = nextOf(next);
  next.offerSkip = rating === 'easy' && next.next?.kind !== 'practice';
  return next;
}

function useInvalidateLearning() {
  const qc = useQueryClient();
  return (track?: string) => {
    void qc.invalidateQueries({ queryKey: keys.dashboard });
    void qc.invalidateQueries({ queryKey: keys.history });
    if (track) void qc.invalidateQueries({ queryKey: keys.track(track) });
  };
}

interface DoneVars {
  ref: string;
  rating: Rating;
  worth: boolean | null;
  track?: string;
  chapter?: string;
}

export function useDone() {
  const qc = useQueryClient();
  const invalidate = useInvalidateLearning();
  return useMutation({
    mutationFn: (v: DoneVars) => api.done(v.ref, v.rating, v.worth),
    onMutate: async (v) => {
      if (!v.track || !v.chapter) return { prev: undefined };
      const key = keys.chapter(v.track, v.chapter);
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<ChapterView>(key);
      if (prev) qc.setQueryData(key, markDone(prev, v.ref, v.rating));
      return { prev };
    },
    onError: (_e, v, ctx) => {
      if (ctx?.prev && v.track && v.chapter)
        qc.setQueryData(keys.chapter(v.track, v.chapter), ctx.prev);
    },
    onSuccess: (res) => {
      if (res.chapter)
        qc.setQueryData(
          keys.chapter(res.chapter.track, res.chapter.chapter),
          res.chapter,
        );
    },
    onSettled: (_r, _e, v) => invalidate(v.track),
  });
}

/** For undo, dismiss and skip: the server answers with the chapter. */
function useChapterChange<V>(fn: (v: V) => Promise<ChapterView | null>) {
  const qc = useQueryClient();
  const invalidate = useInvalidateLearning();
  return useMutation({
    mutationFn: fn,
    onSuccess: (ch) => {
      if (ch) qc.setQueryData(keys.chapter(ch.track, ch.chapter), ch);
      invalidate(ch?.track);
    },
  });
}

export const useUndo = () =>
  useChapterChange((ref: string) => api.undo(ref).then((r) => r.chapter));
export const useDismiss = () =>
  useChapterChange((ref: string) => api.dismiss(ref).then((r) => r.chapter));
export const useSkipToPractice = () =>
  useChapterChange((v: { track: string; chapter: string }) =>
    api.skipToPractice(v.track, v.chapter),
  );

export function useCheckpoint() {
  const qc = useQueryClient();
  const invalidate = useInvalidateLearning();
  return useMutation({
    mutationFn: (v: {
      track: string;
      chapter: string;
      met: boolean[];
      rating: Rating;
      worth: boolean | null;
      force: boolean;
    }) =>
      api.checkpoint(v.track, v.chapter, {
        met: v.met,
        rating: v.rating,
        worth: v.worth ?? undefined,
        force: v.force,
      }),
    onSuccess: (res) => {
      qc.setQueryData(
        keys.chapter(res.chapter.track, res.chapter.chapter),
        res.chapter,
      );
      invalidate(res.chapter.track);
    },
  });
}

export function useSnooze() {
  const invalidate = useInvalidateLearning();
  return useMutation({
    mutationFn: (v: { track: string; chapter: string }) =>
      api.snooze(v.track, v.chapter),
    onSuccess: () => invalidate(),
  });
}

export function useSaveSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (s: Settings) => api.saveSettings(s),
    onSuccess: (s) => {
      qc.setQueryData(keys.settings, s);
      void qc.invalidateQueries({ queryKey: keys.dashboard });
    },
  });
}

export function useAssess() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (a: Record<string, Level>) => api.assess(a),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.assessment });
      void qc.invalidateQueries({ queryKey: keys.dashboard });
    },
  });
}
