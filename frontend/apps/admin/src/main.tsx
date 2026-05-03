import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import App from './App';
import { setUnauthorizedHandler } from './lib/api';
import { useAuthStore } from './stores/auth';
import { toast } from './stores/toast';
import '@kleinai/theme/tokens.css';
import '@kleinai/theme/animations.css';
import './index.css';

const qc = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 30_000 },
  },
});

setUnauthorizedHandler(() => {
  useAuthStore.getState().logout();
  toast.error('登录已过期，请重新登录');
  if (typeof window !== 'undefined' && !window.location.pathname.endsWith('/login')) {
    window.location.href = '/login';
  }
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
