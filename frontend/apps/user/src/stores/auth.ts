// 用户认证 store：token + 当前用户信息。
import { create } from 'zustand';

import { clearToken, loadToken, saveToken, type StoredToken } from '../lib/api';
import { authApi } from '../lib/services';
import type { MeResp, TokenPair } from '../lib/types';

interface AuthState {
  token: StoredToken | null;
  me: MeResp | null;
  loading: boolean;
  setToken: (tok: TokenPair | null) => void;
  setMe: (m: MeResp | null) => void;
  refreshMe: () => Promise<MeResp | null>;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  token: loadToken(),
  me: null,
  loading: false,

  setToken: (tok) => {
    if (!tok) {
      clearToken();
      set({ token: null, me: null });
      return;
    }
    set({ token: saveToken(tok) });
  },

  setMe: (m) => set({ me: m }),

  refreshMe: async () => {
    if (!get().token) return null;
    set({ loading: true });
    try {
      const me = await authApi.me();
      set({ me, loading: false });
      return me;
    } catch {
      set({ loading: false });
      return null;
    }
  },

  logout: async () => {
    try {
      await authApi.logout();
    } catch {
      // ignore
    }
    clearToken();
    set({ token: null, me: null });
  },
}));

export const isAuthed = () => !!useAuthStore.getState().token;
