// 极简 toast 通知 store。
import { create } from 'zustand';

export type ToastKind = 'success' | 'error' | 'info';

export interface Toast {
  id: number;
  kind: ToastKind;
  msg: string;
}

interface ToastState {
  items: Toast[];
  push: (kind: ToastKind, msg: string) => void;
  dismiss: (id: number) => void;
}

let _seq = 1;

export const useToastStore = create<ToastState>((set) => ({
  items: [],
  push: (kind, msg) => {
    const id = _seq++;
    set((s) => ({ items: [...s.items, { id, kind, msg }] }));
    setTimeout(() => {
      set((s) => ({ items: s.items.filter((t) => t.id !== id) }));
    }, 3500);
  },
  dismiss: (id) => set((s) => ({ items: s.items.filter((t) => t.id !== id) })),
}));

export const toast = {
  success: (msg: string) => useToastStore.getState().push('success', msg),
  error: (msg: string) => useToastStore.getState().push('error', msg),
  info: (msg: string) => useToastStore.getState().push('info', msg),
};
