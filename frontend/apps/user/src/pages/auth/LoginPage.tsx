import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import clsx from 'clsx';

import { ApiError } from '../../lib/api';
import { authApi } from '../../lib/services';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

const schema = z.object({
  account: z.string().min(3, '账号至少 3 位'),
  password: z.string().min(6, '密码至少 6 位'),
  remember: z.boolean().default(true),
});

type FormValues = z.infer<typeof schema>;

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const setToken = useAuthStore((s) => s.setToken);
  const refreshMe = useAuthStore((s) => s.refreshMe);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { account: '', password: '', remember: true },
  });

  const onSubmit = async (values: FormValues) => {
    try {
      const resp = await authApi.login({ account: values.account, password: values.password });
      setToken(resp.token);
      await refreshMe();
      toast.success('登录成功');
      const from = (location.state as { from?: string } | null)?.from ?? '/create/image';
      navigate(from, { replace: true });
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '登录失败，请重试';
      toast.error(msg);
    }
  };

  return (
    <div className="space-y-7">
      <header className="space-y-2">
        <h1 className="text-h1 text-text-primary">欢迎回来</h1>
        <p className="text-body text-text-secondary">登录后开启你的 AIGC 创作</p>
      </header>

      <form className="space-y-4" onSubmit={handleSubmit(onSubmit)} noValidate>
        <div className="field">
          <label htmlFor="account" className="field-label">账号</label>
          <input
            id="account"
            placeholder="邮箱 / 手机号 / 用户名"
            autoComplete="username"
            inputMode="email"
            className={clsx('input', errors.account && 'input-error')}
            {...register('account')}
          />
          {errors.account && <p className="field-error">{errors.account.message}</p>}
        </div>

        <div className="field">
          <label htmlFor="password" className="field-label">密码</label>
          <input
            id="password"
            type="password"
            placeholder="请输入密码"
            autoComplete="current-password"
            className={clsx('input', errors.password && 'input-error')}
            {...register('password')}
          />
          {errors.password && <p className="field-error">{errors.password.message}</p>}
        </div>

        <div className="flex items-center justify-between text-small">
          <label className="flex items-center gap-2 text-text-secondary cursor-pointer select-none">
            <input type="checkbox" className="checkbox" {...register('remember')} />
            记住我
          </label>
          <Link to="/forgot" className="text-klein-500 hover:underline">忘记密码?</Link>
        </div>

        <button type="submit" className="btn btn-primary btn-lg btn-block" disabled={isSubmitting}>
          {isSubmitting ? '登录中…' : '登 录'}
        </button>

        <div className="relative my-2">
          <div className="absolute inset-0 grid place-items-center">
            <div className="h-px w-full bg-border" />
          </div>
          <div className="relative flex justify-center">
            <span className="bg-surface-bg px-3 text-tiny text-text-tertiary uppercase tracking-wider">或者</span>
          </div>
        </div>

        <button type="button" className="btn btn-outline btn-lg btn-block" disabled>
          使用 微信 登录（敬请期待）
        </button>
      </form>

      <p className="text-small text-text-secondary text-center">
        还没账号？
        <Link to="/register" className="text-klein-500 hover:underline ml-1">立即注册</Link>
      </p>
    </div>
  );
}
