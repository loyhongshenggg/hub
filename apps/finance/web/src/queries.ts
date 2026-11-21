// Server state (TanStack Query). Mutations invalidate what they affect.
import {
  QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { api, type Catalog, type Category, type Goal, type TxnPatch, type TxnQuery } from './api';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 20_000, refetchOnWindowFocus: true, retry: 1 },
  },
});

export const keys = {
  status: ['status'] as const,
  catalog: ['catalog'] as const,
  overview: (m?: string, spread?: boolean) => ['overview', m ?? '', !!spread] as const,
  txns: (q: TxnQuery) => ['txns', q] as const,
  statements: ['statements'] as const,
  statement: (id: number) => ['statement', id] as const,
  goals: ['goals'] as const,
  goal: (id: number) => ['goal', id] as const,
  claude: ['claude'] as const,
  rules: ['rules'] as const,
};

export const useStatus = () =>
  useQuery({ queryKey: keys.status, queryFn: api.status, refetchInterval: 15_000 });

export const useCatalog = () =>
  useQuery({ queryKey: keys.catalog, queryFn: api.catalog, staleTime: Infinity });

/** Category lookup that never fails (unknown ids → Other). */
export function useCategories() {
  const { data } = useCatalog();
  return catalogLookup(data);
}

const cache = new WeakMap<Catalog, (id: string) => Category & { slot: number }>();
function catalogLookup(data: Catalog | undefined) {
  if (!data) return { list: [] as Category[], get: fallbackGet, groups: [] as Catalog['groups'] };
  let get = cache.get(data);
  if (!get) {
    const byId = new Map(data.categories.map((c) => [c.id, c]));
    const slot = new Map(data.groups.map((g) => [g.id, g.slot]));
    get = (id: string) => {
      const c = byId.get(id) ?? { id: 'other', name: id ? id : 'Uncategorised', group: 'other', essential: false };
      return { ...c, slot: slot.get(c.group) ?? 0 };
    };
    cache.set(data, get);
  }
  return { list: data.categories, get, groups: data.groups };
}

function fallbackGet(id: string) {
  return { id, name: id || 'Uncategorised', group: 'other', essential: false, slot: 0 };
}

export const useOverview = (month?: string, spread?: boolean) =>
  useQuery({
    queryKey: keys.overview(month, spread),
    queryFn: () => api.overview(month, spread),
    placeholderData: (prev) => prev,
  });

export const useTxns = (q: TxnQuery, enabled = true) =>
  useQuery({ queryKey: keys.txns(q), queryFn: () => api.txns(q), enabled, placeholderData: (p) => p });

export const useStatements = () =>
  useQuery({
    queryKey: keys.statements,
    queryFn: api.statements,
    // Poll while something is being read.
    refetchInterval: (q) =>
      q.state.data?.some((s) => s.status === 'queued' || s.status === 'reading') ? 1500 : false,
  });

export const useStatement = (id: number) =>
  useQuery({
    queryKey: keys.statement(id),
    queryFn: () => api.statement(id),
    refetchInterval: (q) => {
      const s = q.state.data?.statement.status;
      return s === 'queued' || s === 'reading' ? 1500 : false;
    },
  });

export const useGoals = () => useQuery({ queryKey: keys.goals, queryFn: api.goals });
export const useGoal = (id: number) =>
  useQuery({ queryKey: keys.goal(id), queryFn: () => api.goal(id) });
export const useClaude = () => useQuery({ queryKey: keys.claude, queryFn: api.claude });
export const useRules = () => useQuery({ queryKey: keys.rules, queryFn: api.rules });

/** invalidateLedger refreshes everything derived from transactions. */
export function useInvalidateLedger() {
  const qc = useQueryClient();
  return () => {
    for (const k of ['overview', 'txns', 'goals', 'goal', 'status', 'statement', 'statements', 'rules', 'people', 'split', 'person']) {
      void qc.invalidateQueries({ queryKey: [k] });
    }
  };
}

export function usePatchTxn() {
  const inv = useInvalidateLedger();
  return useMutation({
    mutationFn: ({ id, patch }: { id: number; patch: TxnPatch }) => api.patchTxn(id, patch),
    onSuccess: inv,
  });
}

export function useSaveGoal() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (g: Partial<Goal> & { id?: number }) =>
      g.id ? api.updateGoal(g as Goal) : api.createGoal(g),
    onSuccess: (v) => {
      qc.setQueryData(keys.goal(v.id), v);
      void qc.invalidateQueries({ queryKey: keys.goals });
    },
  });
}
