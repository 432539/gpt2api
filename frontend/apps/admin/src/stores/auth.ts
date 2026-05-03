// 后台认证 store。
import { create } from 'zustand';

import { clearToken, loadToken, saveToken, type StoredToken } from '../lib/api';
import { authApi } from '../lib/services';
import type { AdminLoginResp, AdminMe } from '../lib/types';

interface AuthState {
  token: StoredToken | null;
  me: AdminMe | null;
  loading: boolean;
  setLogin: (resp: AdminLoginResp) => void;
  setMe: (m: AdminMe | null) => void;
  refreshMe: () => Promise<AdminMe | null>;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  token: loadToken(),
  me: null,
  loading: false,

  setLogin: (resp) => {
    const tok = saveToken(resp.token);
    set({
      token: tok,
      me: {
        id: resp.id,
        username: resp.username,
        nickname: resp.nickname,
        role_id: resp.role_id,
        role_code: '',
        role_name: '',
      },
    });
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

  logout: () => {
    clearToken();
    set({ token: null, me: null });
  },
}));

export const isAuthed = () => !!useAuthStore.getState().token;
