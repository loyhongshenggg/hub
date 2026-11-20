import { create } from 'zustand';

export interface Toast {
  id: number;
  message: string;
}

interface ToastState {
  toasts: Toast[];
  dismiss: (id: number) => void;
}

let next = 1;

export const useToasts = create<ToastState>((set) => ({
  toasts: [],
  dismiss: (id) =>
    set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}));

/** Show a short message bottom-right for a few seconds. */
export function toast(message: string) {
  const id = next++;
  useToasts.setState((s) => ({ toasts: [...s.toasts, { id, message }] }));
  setTimeout(() => useToasts.getState().dismiss(id), 6000);
}
