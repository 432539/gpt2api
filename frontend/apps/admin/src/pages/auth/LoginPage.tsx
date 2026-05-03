import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { useLocation, useNavigate } from 'react-router-dom';
import { z } from 'zod';
import clsx from 'clsx';

import { Logo } from '../../components/Logo';
import { ApiError } from '../../lib/api';
import { authApi } from '../../lib/services';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

const schema = z.object({
  username: z.string().min(2, '账号至少 2 位').max(64, '账号过长'),
  password: z.string().min(6, '密码至少 6 位').max(72, '密码过长'),
});

type FormBody = z.infer<typeof schema>;

interface LocState {
  from?: { pathname?: string };
}

export default function LoginPage() {
  const nav = useNavigate();
  const loc = useLocation();
  const setLogin = useAuthStore((s) => s.setLogin);
  const refreshMe = useAuthStore((s) => s.refreshMe);

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormBody>({
    resolver: zodResolver(schema),
    defaultValues: { username: '', password: '' },
  });

  const m = useMutation({
    mutationFn: (b: FormBody) => authApi.login(b.username, b.password),
    onSuccess: async (resp) => {
      setLogin(resp);
      void refreshMe();
      toast.success(`欢迎回来，${resp.nickname || resp.username}`);
      const target = (loc.state as LocState | null)?.from?.pathname || '/dashboard';
      nav(target, { replace: true });
    },
    onError: (err: ApiError) => {
      toast.error(err.message || '登录失败');
    },
  });

  return (
    <div className="grid min-h-screen place-items-center bg-klein-gradient px-4 py-10">
      <form
        onSubmit={handleSubmit((v) => m.mutate(v))}
        className="dialog-surface w-full max-w-sm p-6 sm:p-8 space-y-5"
      >
        <header className="text-center space-y-3">
          <div className="flex justify-center">
            <Logo size="lg" suffix="管理后台" />
          </div>
          <p className="text-small text-text-secondary">仅授权 IP 可访问</p>
        </header>

        <div className="field">
          <label className="field-label">账号</label>
          <input
            className={clsx('input', errors.username && 'input-error')}
            placeholder="管理员账号"
            autoComplete="username"
            {...register('username')}
          />
          {errors.username && <p className="field-error">{errors.username.message}</p>}
        </div>

        <div className="field">
          <label className="field-label">密码</label>
          <input
            className={clsx('input', errors.password && 'input-error')}
            type="password"
            placeholder="密码"
            autoComplete="current-password"
            {...register('password')}
          />
          {errors.password && <p className="field-error">{errors.password.message}</p>}
        </div>

        <button type="submit" className="btn btn-primary btn-lg btn-block" disabled={m.isPending}>
          {m.isPending ? '登录中…' : '登 录'}
        </button>

        <p className="text-center text-tiny text-text-tertiary">
          初始账号 <code className="kbd">admin / admin123</code> · 登录后请立即修改
        </p>
      </form>
    </div>
  );
}
