import { type FormEvent, useState } from 'react';
import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import {
  BadgeDollarSign,
  ChevronDown,
  FileText,
  Globe2,
  KeyRound,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  Menu,
  ReceiptText,
  Settings,
  Tag,
  Ticket,
  UserCircle2,
  Users,
  Wallet,
  WalletCards,
  X,
} from 'lucide-react';
import clsx from 'clsx';

import { Logo } from '../components/Logo';
import { authApi } from '../lib/services';
import { useAuthStore } from '../stores/auth';
import { toast } from '../stores/toast';

const APP_VERSION = 'v2.0.1';

const NAV = [
  { to: '/dashboard', label: '仪表盘', icon: LayoutDashboard },
  { to: '/accounts', label: 'Token 管理', icon: KeyRound },
  { to: '/proxies', label: '代理管理', icon: Globe2 },
  { to: '/users', label: '用户管理', icon: Users },
  { to: '/billing', label: '充值消费', icon: Wallet },
  { to: '/promo', label: '优惠码', icon: Tag },
  { to: '/cdk', label: '兑换码 CDK', icon: Ticket },
  { to: '/config', label: '系统配置', icon: Settings },
  { to: '/billing-settings', label: '扣费设置', icon: ReceiptText },
  { to: '/recharge-packages', label: '充值套餐', icon: WalletCards },
  { to: '/model-prices', label: '模型价格', icon: BadgeDollarSign },
  { to: '/logs', label: '请求日志', icon: FileText },
] as const;

export function AdminLayout() {
  const [mobileOpen, setMobileOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [passwordOpen, setPasswordOpen] = useState(false);
  const me = useAuthStore((s) => s.me);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    toast.info('已退出登录');
    navigate('/login', { replace: true });
  };

  const displayName = me?.nickname || me?.username || '管理员';
  const roleName = me?.role_name || me?.role_code || '管理员';
  const initial = displayName.slice(0, 1).toUpperCase();

  return (
    <div className="grid min-h-screen bg-surface-bg lg:grid-cols-[240px_1fr]">
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-3 border-b border-border bg-surface-1 px-4 lg:hidden">
        <Logo size="sm" suffix="管理后台" />
        <button
          aria-label="打开菜单"
          className="btn btn-ghost btn-icon btn-sm"
          onClick={() => setMobileOpen((v) => !v)}
        >
          {mobileOpen ? <X size={20} /> : <Menu size={20} />}
        </button>
      </header>

      <aside
        className={clsx(
          'flex-col border-r border-border bg-surface-1 lg:sticky lg:top-0 lg:flex lg:h-screen',
          mobileOpen ? 'fixed inset-y-0 left-0 z-40 flex w-[260px] shadow-3' : 'hidden',
        )}
      >
        <div className="hidden h-16 items-center border-b border-border px-5 lg:flex">
          <Logo suffix="管理后台" />
        </div>
        <nav className="flex-1 space-y-1 overflow-y-auto p-3">
          {NAV.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              onClick={() => setMobileOpen(false)}
              className={({ isActive }) =>
                clsx(
                  'flex h-10 min-w-[44px] items-center gap-3 rounded-md px-3 text-small transition',
                  isActive
                    ? 'bg-klein-gradient text-text-on-klein shadow-glow-soft'
                    : 'text-text-secondary hover:bg-surface-2 hover:text-text-primary',
                )
              }
            >
              <Icon size={18} />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="border-t border-border px-4 py-4 text-tiny text-text-tertiary">
          <div className="flex items-center gap-2">
            <span>内部版本</span>
            <span className="text-text-secondary">{APP_VERSION}</span>
          </div>
        </div>
      </aside>

      {mobileOpen && (
        <button
          type="button"
          aria-label="关闭菜单"
          className="fixed inset-0 z-30 bg-surface-overlay lg:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      <main className="flex min-w-0 flex-col">
        <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-border bg-surface-1/90 px-4 backdrop-blur sm:px-6">
          <h1 className="truncate text-h5 text-text-primary">管理后台</h1>
          <div
            className="relative"
            onBlur={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setMenuOpen(false);
            }}
          >
            <button
              type="button"
              className="inline-flex items-center gap-2 rounded-full px-2 py-1 transition hover:bg-surface-2"
              onClick={() => setMenuOpen((v) => !v)}
            >
              <span className="hidden rounded-full bg-surface-2 px-3 py-1 text-small text-text-secondary sm:inline-flex">
                {roleName}
              </span>
              <span className="grid h-8 w-8 flex-shrink-0 place-items-center rounded-pill bg-klein-gradient text-small text-white">
                {initial}
              </span>
              <ChevronDown size={15} className={clsx('text-text-tertiary transition', menuOpen && 'rotate-180')} />
            </button>

            {menuOpen && (
              <div className="absolute right-0 top-12 z-40 w-[260px] overflow-hidden rounded-xl border border-border bg-surface-1 shadow-4">
                <div className="border-b border-border px-4 py-3">
                  <div className="flex items-center gap-3">
                    <span className="grid h-10 w-10 place-items-center rounded-pill bg-klein-gradient text-white">
                      {initial}
                    </span>
                    <div className="min-w-0">
                      <p className="truncate text-body text-text-primary">{displayName}</p>
                      <p className="truncate text-small text-text-tertiary">{me?.username || 'admin'}</p>
                    </div>
                  </div>
                  <div className="mt-3 inline-flex rounded-full bg-surface-2 px-2.5 py-1 text-tiny text-text-secondary">
                    {roleName}
                  </div>
                </div>
                <button
                  type="button"
                  className="flex h-11 w-full items-center gap-3 px-4 text-left text-small text-text-secondary hover:bg-surface-2 hover:text-text-primary"
                  onClick={() => {
                    setMenuOpen(false);
                    setPasswordOpen(true);
                  }}
                >
                  <LockKeyhole size={16} />
                  修改密码
                </button>
                <button
                  type="button"
                  className="flex h-11 w-full items-center gap-3 px-4 text-left text-small text-text-secondary hover:bg-surface-2 hover:text-text-primary"
                  disabled
                >
                  <UserCircle2 size={16} />
                  账号信息
                </button>
                <button
                  type="button"
                  className="flex h-11 w-full items-center gap-3 border-t border-border px-4 text-left text-small text-danger hover:bg-danger-soft"
                  onClick={handleLogout}
                >
                  <LogOut size={16} />
                  退出登录
                </button>
              </div>
            )}
          </div>
        </header>
        <div className="min-w-0 flex-1">
          <Outlet />
        </div>
      </main>

      {passwordOpen && <PasswordDialog onClose={() => setPasswordOpen(false)} />}
    </div>
  );
}

function PasswordDialog({ onClose }: { onClose: () => void }) {
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [saving, setSaving] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (newPassword.length < 8) {
      toast.error('新密码至少 8 位');
      return;
    }
    if (newPassword !== confirm) {
      toast.error('两次输入的新密码不一致');
      return;
    }
    setSaving(true);
    try {
      await authApi.changePassword({ old_password: oldPassword, new_password: newPassword });
      toast.success('密码已修改');
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '修改失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-surface-overlay px-4">
      <button className="absolute inset-0" type="button" aria-label="关闭" onClick={onClose} />
      <form className="dialog-surface relative w-full max-w-md p-6" onSubmit={submit}>
        <div className="mb-5 flex items-center justify-between">
          <div>
            <h2 className="text-h3 text-text-primary">修改密码</h2>
            <p className="mt-1 text-small text-text-tertiary">建议及时修改默认管理员密码。</p>
          </div>
          <button className="btn btn-ghost btn-icon btn-sm" type="button" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="space-y-3">
          <label className="field">
            <span className="field-label">原密码</span>
            <input className="input" type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} autoComplete="current-password" />
          </label>
          <label className="field">
            <span className="field-label">新密码</span>
            <input className="input" type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} autoComplete="new-password" />
          </label>
          <label className="field">
            <span className="field-label">确认新密码</span>
            <input className="input" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" />
          </label>
        </div>
        <div className="mt-6 flex justify-end gap-2">
          <button className="btn btn-outline btn-md" type="button" onClick={onClose}>
            取消
          </button>
          <button className="btn btn-primary btn-md" type="submit" disabled={saving}>
            {saving ? '保存中…' : '保存修改'}
          </button>
        </div>
      </form>
    </div>
  );
}
