import { Link, useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import clsx from 'clsx';

import { ApiError } from '../../lib/api';
import { authApi } from '../../lib/services';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

const schema = z
  .object({
    account: z.string().min(3, '账号至少 3 位').max(64, '账号过长'),
    password: z
      .string()
      .min(8, '密码至少 8 位')
      .max(64, '密码过长')
      .regex(/[A-Za-z]/, '密码需包含字母')
      .regex(/[0-9]/, '密码需包含数字'),
    confirm: z.string(),
    invite_code: z.string().max(16).optional().or(z.literal('')),
  })
  .refine((d) => d.password === d.confirm, {
    message: '两次密码不一致',
    path: ['confirm'],
  });

type FormValues = z.infer<typeof schema>;

export default function RegisterPage() {
  const navigate = useNavigate();
  const setToken = useAuthStore((s) => s.setToken);
  const refreshMe = useAuthStore((s) => s.refreshMe);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { account: '', password: '', confirm: '', invite_code: '' },
  });

  const onSubmit = async (values: FormValues) => {
    try {
      const resp = await authApi.register({
        account: values.account,
        password: values.password,
        invite_code: values.invite_code || undefined,
      });
      setToken(resp.token);
      await refreshMe();
      toast.success('注册成功，已为你登录');
      navigate('/create/image', { replace: true });
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '注册失败，请重试';
      toast.error(msg);
    }
  };

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <h1 className="text-h1 text-text-primary">注册账号</h1>
        <p className="text-body text-text-secondary">创建账号开启你的 AIGC 之旅</p>
      </header>

      <form className="space-y-4" onSubmit={handleSubmit(onSubmit)} noValidate>
        <div className="field">
          <label className="field-label">账号</label>
          <input
            className={clsx('input', errors.account && 'input-error')}
            placeholder="邮箱 / 手机号 / 用户名"
            autoComplete="username"
            {...register('account')}
          />
          {errors.account && <p className="field-error">{errors.account.message}</p>}
        </div>

        <div className="field">
          <label className="field-label">设置密码</label>
          <input
            className={clsx('input', errors.password && 'input-error')}
            type="password"
            placeholder="≥ 8 位，含字母与数字"
            autoComplete="new-password"
            {...register('password')}
          />
          {errors.password && <p className="field-error">{errors.password.message}</p>}
        </div>

        <div className="field">
          <label className="field-label">确认密码</label>
          <input
            className={clsx('input', errors.confirm && 'input-error')}
            type="password"
            placeholder="再次输入密码"
            autoComplete="new-password"
            {...register('confirm')}
          />
          {errors.confirm && <p className="field-error">{errors.confirm.message}</p>}
        </div>

        <div className="field">
          <label className="field-label">邀请码（选填）</label>
          <input className="input" placeholder="填写以获得额外点数" {...register('invite_code')} />
          <p className="field-hint">使用邀请码注册可获得额外赠点。</p>
        </div>

        <button className="btn btn-primary btn-lg btn-block" type="submit" disabled={isSubmitting}>
          {isSubmitting ? '创建中…' : '创 建 账 号'}
        </button>

        <p className="text-small text-text-tertiary text-center">
          注册即代表同意 <a className="text-klein-500">服务条款</a> 与 <a className="text-klein-500">隐私政策</a>
        </p>
      </form>

      <p className="text-small text-text-secondary text-center">
        已有账号？
        <Link to="/login" className="text-klein-500 hover:underline ml-1">立即登录</Link>
      </p>
    </div>
  );
}
