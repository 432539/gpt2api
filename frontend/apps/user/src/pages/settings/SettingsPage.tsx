import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import clsx from 'clsx';

import { applyThemeMode, type ThemeMode } from '@kleinai/theme';

import { ApiError } from '../../lib/api';
import { fmtPoints, fmtTime } from '../../lib/format';
import { authApi } from '../../lib/services';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

const pwdSchema = z
  .object({
    old_password: z.string().min(6, '原密码至少 6 位'),
    new_password: z
      .string()
      .min(8, '新密码至少 8 位')
      .regex(/[A-Za-z]/, '需包含字母')
      .regex(/[0-9]/, '需包含数字'),
    confirm: z.string(),
  })
  .refine((d) => d.new_password === d.confirm, {
    message: '两次密码不一致',
    path: ['confirm'],
  });

type PwdForm = z.infer<typeof pwdSchema>;

export default function SettingsPage() {
  const me = useAuthStore((s) => s.me);
  const [mode, setLocalMode] = useState<ThemeMode>(
    (localStorage.getItem('klein:theme') as ThemeMode | null) ?? 'light',
  );

  const setTheme = (m: ThemeMode) => {
    applyThemeMode(m);
    localStorage.setItem('klein:theme', m);
    setLocalMode(m);
  };

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<PwdForm>({
    resolver: zodResolver(pwdSchema),
    defaultValues: { old_password: '', new_password: '', confirm: '' },
  });

  const pwdMut = useMutation({
    mutationFn: (body: { old_password: string; new_password: string }) =>
      authApi.changePassword(body),
    onSuccess: () => {
      toast.success('密码修改成功');
      reset();
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '修改失败'),
  });

  const themeOptions: { value: ThemeMode; label: string; desc: string }[] = [
    { value: 'light', label: '浅色', desc: '默认界面 · 简洁' },
    { value: 'dark', label: '深色', desc: '高对比显示' },
    { value: 'system', label: '跟随系统', desc: '自动切换' },
  ];

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1 className="page-title">个人设置</h1>
          <p className="page-subtitle">基础资料 · 主题偏好 · 安全设置</p>
        </div>
      </header>

      <section className="card card-section mb-4">
        <h3 className="section-title mb-4">资料</h3>
        <div className="grid sm:grid-cols-2 gap-3">
          <Field label="UID" value={me?.uid?.toString() ?? '—'} />
          <Field label="UUID" value={me?.uuid ?? '—'} mono />
          <Field label="账号" value={me?.username || me?.email || me?.phone || '—'} />
          <Field label="邀请码" value={me?.invite_code ?? '—'} mono />
          <Field label="套餐" value={me?.plan_code?.toUpperCase() ?? 'FREE'} />
          <Field label="可用点数" value={fmtPoints(me?.points ?? 0)} />
          <Field label="冻结点数" value={fmtPoints(me?.frozen_points ?? 0)} />
          <Field label="注册时间" value={fmtTime(me?.created_at ?? 0)} />
        </div>
      </section>

      <section className="card card-section mb-4">
        <h3 className="section-title mb-4">主题偏好</h3>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
          {themeOptions.map((o) => (
            <button
              key={o.value}
              className={clsx(
                'rounded-md border px-4 py-3 text-left transition',
                mode === o.value
                  ? 'border-klein-500 bg-klein-gradient-soft text-text-primary shadow-1'
                  : 'border-border bg-surface-1 hover:border-border-strong',
              )}
              onClick={() => setTheme(o.value)}
            >
              <p className="font-semibold text-body">{o.label}</p>
              <p className="text-small text-text-tertiary mt-0.5">{o.desc}</p>
            </button>
          ))}
        </div>
      </section>

      <section className="card card-section">
        <h3 className="section-title mb-4">修改密码</h3>
        <form
          className="grid sm:grid-cols-2 gap-3"
          onSubmit={handleSubmit((d) =>
            pwdMut.mutate({ old_password: d.old_password, new_password: d.new_password }),
          )}
          noValidate
        >
          <div className="field sm:col-span-2">
            <label className="field-label">原密码</label>
            <input
              type="password"
              className={clsx('input', errors.old_password && 'input-error')}
              autoComplete="current-password"
              {...register('old_password')}
            />
            {errors.old_password && <p className="field-error">{errors.old_password.message}</p>}
          </div>
          <div className="field">
            <label className="field-label">新密码</label>
            <input
              type="password"
              className={clsx('input', errors.new_password && 'input-error')}
              autoComplete="new-password"
              {...register('new_password')}
            />
            {errors.new_password && <p className="field-error">{errors.new_password.message}</p>}
          </div>
          <div className="field">
            <label className="field-label">确认密码</label>
            <input
              type="password"
              className={clsx('input', errors.confirm && 'input-error')}
              autoComplete="new-password"
              {...register('confirm')}
            />
            {errors.confirm && <p className="field-error">{errors.confirm.message}</p>}
          </div>
          <div className="sm:col-span-2 flex justify-end">
            <button className="btn btn-primary btn-lg" disabled={pwdMut.isPending}>
              {pwdMut.isPending ? '保存中…' : '保存修改'}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-md bg-surface-2 border border-border px-4 py-3">
      <p className="text-tiny text-text-tertiary uppercase tracking-wider">{label}</p>
      <p className={clsx('mt-1 text-text-primary break-all', mono ? 'font-mono text-small' : 'text-body font-medium')}>
        {value}
      </p>
    </div>
  );
}
