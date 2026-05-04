import type { MouseEvent } from 'react';
import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import {
  BookOpen,
  Clock3,
  CreditCard,
  FileKey2,
  Gift,
  Image,
  LogIn,
  LogOut,
  MessageCircle,
  PanelLeft,
  Search,
  Settings,
  Video,
  type LucideIcon,
} from 'lucide-react';
import clsx from 'clsx';

import { useAuthStore } from '../stores/auth';
import { useLoginGateStore } from '../stores/loginGate';
import { toast } from '../stores/toast';

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  authed?: boolean;
}

const APP_VERSION = 'v2.0.1';

const NAV_ITEMS: NavItem[] = [
  { to: '/create/image', label: '图片', icon: Image },
  { to: '/create/text', label: '文字', icon: MessageCircle },
  { to: '/create/video', label: '视频', icon: Video },
  { to: '/history', label: '历史', icon: Clock3, authed: true },
  { to: '/billing', label: '充值', icon: CreditCard, authed: true },
  { to: '/keys', label: '密钥', icon: FileKey2, authed: true },
  { to: '/docs', label: '文档', icon: BookOpen },
  { to: '/invite', label: '邀请', icon: Gift, authed: true },
  { to: '/settings', label: '设置', icon: Settings, authed: true },
];

export function AppLayout() {
  const token = useAuthStore((s) => s.token);
  const me = useAuthStore((s) => s.me);
  const logout = useAuthStore((s) => s.logout);
  const openGate = useLoginGateStore((s) => s.openGate);
  const navigate = useNavigate();
  const isAuthed = !!token;

  const onLogout = async () => {
    await logout();
    toast.info('已退出登录');
    navigate('/create/image', { replace: true });
  };

  const handleNav = (item: NavItem, e: MouseEvent) => {
    if (item.authed && !isAuthed) {
      e.preventDefault();
      openGate({ hint: `登录后即可使用“${item.label}”`, onLoggedIn: () => navigate(item.to) });
    }
  };

  return (
    <div className="min-h-full bg-white text-neutral-950">
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-14 border-r border-neutral-200 bg-white lg:flex lg:flex-col lg:items-center">
        <button
          type="button"
          className="mt-3 grid h-8 w-8 place-items-center rounded-full text-neutral-700 hover:bg-neutral-100"
          title="首页"
          onClick={() => navigate('/create/image')}
        >
          <PanelLeft size={18} />
        </button>

        <nav className="mt-6 flex flex-1 flex-col items-center gap-2">
          {NAV_ITEMS.slice(0, 4).map((item) => <RailLink key={item.to} item={item} onClick={handleNav} />)}
          <div className="my-2 h-px w-6 bg-neutral-200" />
          {NAV_ITEMS.slice(4, 7).map((item) => <RailLink key={item.to} item={item} onClick={handleNav} />)}
        </nav>

        <div className="mb-3 flex flex-col items-center gap-2">
          {NAV_ITEMS.slice(7).map((item) => <RailLink key={item.to} item={item} onClick={handleNav} />)}
          {isAuthed ? (
            <>
              <button
                type="button"
                className="grid h-8 w-8 place-items-center rounded-full bg-emerald-500 text-xs font-semibold text-white"
                title={me?.username || me?.email || '我的账号'}
                onClick={() => navigate('/settings')}
              >
                {(me?.username || me?.email || 'U').slice(0, 1).toUpperCase()}
              </button>
              <button
                type="button"
                className="grid h-8 w-8 place-items-center rounded-full text-neutral-600 hover:bg-neutral-100"
                title="退出登录"
                onClick={onLogout}
              >
                <LogOut size={17} />
              </button>
            </>
          ) : (
            <button
              type="button"
              className="grid h-8 w-8 place-items-center rounded-full text-neutral-600 hover:bg-neutral-100"
              title="登录"
              onClick={() => openGate({ hint: '登录后可保存作品和查看额度' })}
            >
              <LogIn size={17} />
            </button>
          )}
        </div>
        <div className="mb-2 flex flex-col items-center gap-1 text-[11px] text-neutral-400">
          <span>{APP_VERSION}</span>
        </div>
      </aside>

      <header className="sticky top-0 z-30 flex h-12 items-center justify-between border-b border-neutral-200 bg-white/90 px-3 backdrop-blur lg:hidden">
        <button className="grid h-9 w-9 place-items-center rounded-full hover:bg-neutral-100" onClick={() => navigate('/create/image')}>
          <PanelLeft size={18} />
        </button>
        <div className="flex items-center gap-1">
          {NAV_ITEMS.slice(0, 3).map((item) => <MobileMode key={item.to} item={item} onClick={handleNav} />)}
        </div>
        <button className="grid h-9 w-9 place-items-center rounded-full hover:bg-neutral-100" onClick={() => navigate('/history')}>
          <Search size={18} />
        </button>
      </header>

      <main className="min-h-screen lg:pl-14">
        <Outlet />
      </main>
    </div>
  );
}

function RailLink({ item, onClick }: { item: NavItem; onClick: (item: NavItem, e: MouseEvent) => void }) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      title={item.label}
      onClick={(e) => onClick(item, e)}
      className={({ isActive }) =>
        clsx(
          'grid h-9 w-9 place-items-center rounded-full transition',
          isActive ? 'bg-neutral-950 text-white' : 'text-neutral-650 hover:bg-neutral-100 hover:text-neutral-950',
        )
      }
    >
      <Icon size={18} />
    </NavLink>
  );
}

function MobileMode({ item, onClick }: { item: NavItem; onClick: (item: NavItem, e: MouseEvent) => void }) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      onClick={(e) => onClick(item, e)}
      className={({ isActive }) =>
        clsx(
          'inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-sm',
          isActive ? 'bg-neutral-950 text-white' : 'text-neutral-700 hover:bg-neutral-100',
        )
      }
    >
      <Icon size={15} />
      {item.label}
    </NavLink>
  );
}
