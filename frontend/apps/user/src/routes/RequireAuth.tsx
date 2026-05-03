import { useEffect } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';

import { LoadingScreen } from '../components/LoadingScreen';
import { useAuthStore } from '../stores/auth';
import { useLoginGateStore } from '../stores/loginGate';

/**
 * 受保护页面外层。未登录时：
 *   1. 弹出登录浮层（hint = 该页面需要登录）
 *   2. 重定向回首页（/create/image），避免显示半截内容
 * 登录成功后用户可手动再点对应入口。
 */
export function RequireAuth() {
  const token = useAuthStore((s) => s.token);
  const me = useAuthStore((s) => s.me);
  const loading = useAuthStore((s) => s.loading);
  const refreshMe = useAuthStore((s) => s.refreshMe);
  const openGate = useLoginGateStore((s) => s.openGate);
  const loc = useLocation();

  useEffect(() => {
    if (token && !me) {
      void refreshMe();
    }
  }, [token, me, refreshMe]);

  useEffect(() => {
    if (!token) {
      openGate({ hint: '该页面需要登录后访问' });
    }
  }, [token, openGate]);

  if (!token) {
    return <Navigate to="/" replace state={{ from: loc.pathname + loc.search }} />;
  }
  if (!me && loading) {
    return <LoadingScreen />;
  }
  return <Outlet />;
}
