import { Suspense, lazy } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';

import { AppLayout } from './layouts/AppLayout';
import { AuthLayout } from './layouts/AuthLayout';
import { LoadingScreen } from './components/LoadingScreen';
import { LoginGate } from './components/LoginGate';
import { Toaster } from './components/Toaster';
import { RequireAuth } from './routes/RequireAuth';

const LoginPage = lazy(() => import('./pages/auth/LoginPage'));
const RegisterPage = lazy(() => import('./pages/auth/RegisterPage'));
const CreateStudioPage = lazy(() => import('./pages/create/CreateStudioPage'));
const HistoryPage = lazy(() => import('./pages/create/HistoryPage'));
const BillingPage = lazy(() => import('./pages/billing/BillingPage'));
const KeysPage = lazy(() => import('./pages/keys/KeysPage'));
const DocsPage = lazy(() => import('./pages/keys/DocsPage'));
const InvitePage = lazy(() => import('./pages/invite/InvitePage'));
const SettingsPage = lazy(() => import('./pages/settings/SettingsPage'));

export default function App() {
  return (
    <>
      <Toaster />
      <LoginGate />
      <Suspense fallback={<LoadingScreen />}>
        <Routes>
          {/* 独立的全屏品牌登录/注册页（保留兜底入口） */}
          <Route element={<AuthLayout />}>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/register" element={<RegisterPage />} />
          </Route>

          {/* 主应用：未登录也可浏览创作页 / 调用说明 */}
          <Route element={<AppLayout />}>
            <Route path="/" element={<Navigate to="/create/image" replace />} />
            <Route path="/create/image" element={<CreateStudioPage />} />
            <Route path="/create/text" element={<CreateStudioPage />} />
            <Route path="/create/video" element={<CreateStudioPage />} />
            <Route path="/docs" element={<DocsPage />} />

            {/* 受保护：未登录将弹浮层并退回首页 */}
            <Route element={<RequireAuth />}>
              <Route path="/history" element={<HistoryPage />} />
              <Route path="/billing" element={<BillingPage />} />
              <Route path="/keys" element={<KeysPage />} />
              <Route path="/invite" element={<InvitePage />} />
              <Route path="/settings" element={<SettingsPage />} />
            </Route>
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </>
  );
}
