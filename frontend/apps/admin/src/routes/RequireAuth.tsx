import { useEffect } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';

import { useAuthStore } from '../stores/auth';

export default function RequireAuth() {
  const loc = useLocation();
  const token = useAuthStore((s) => s.token);
  const me = useAuthStore((s) => s.me);
  const refreshMe = useAuthStore((s) => s.refreshMe);

  useEffect(() => {
    if (token && !me) {
      void refreshMe();
    }
  }, [token, me, refreshMe]);

  if (!token) {
    return <Navigate to="/login" replace state={{ from: loc }} />;
  }
  if (token && !me) {
    return (
      <div className="grid h-screen place-items-center text-text-tertiary">
        正在加载管理员信息…
      </div>
    );
  }
  return <Outlet />;
}
