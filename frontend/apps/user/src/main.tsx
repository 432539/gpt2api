import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { applyThemeMode } from '@kleinai/theme';

import App from './App';
import { setUnauthorizedHandler } from './lib/api';
import { useAuthStore } from './stores/auth';
import { useLoginGateStore } from './stores/loginGate';
import { toast } from './stores/toast';
import '@kleinai/theme/tokens.css';
import '@kleinai/theme/animations.css';
import './index.css';

applyThemeMode((localStorage.getItem('klein:theme') as 'dark' | 'light' | null) ?? 'dark');

setUnauthorizedHandler(() => {
  useAuthStore.setState({ token: null, me: null });
  toast.error('登录已过期，请重新登录');
  // 让 token 失效的请求触发登录浮层，而不是粗暴跳转
  useLoginGateStore.getState().openGate({ hint: '登录态已失效，请重新登录' });
});

const qc = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
});

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>,
);
