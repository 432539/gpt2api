import { Link, Outlet } from 'react-router-dom';
import { Image, MessageCircle, Video } from 'lucide-react';

import { Logo } from '../components/Logo';

export function AuthLayout() {
  return (
    <div className="min-h-full bg-white text-neutral-950">
      <div className="mx-auto flex min-h-screen w-full max-w-6xl flex-col px-5 py-5">
        <header className="flex items-center justify-between border-b border-neutral-100 pb-4">
          <Link to="/create/image" className="inline-flex items-center gap-2 text-neutral-900">
            <Logo size="sm" />
          </Link>
          <nav className="hidden items-center gap-2 rounded-full bg-neutral-100 p-1 text-sm text-neutral-500 sm:flex">
            <Link className="inline-flex h-9 items-center gap-2 rounded-full bg-white px-4 text-neutral-950 shadow-sm" to="/create/image">
              <Image size={15} /> 图片
            </Link>
            <Link className="inline-flex h-9 items-center gap-2 rounded-full px-4 hover:text-neutral-950" to="/create/text">
              <MessageCircle size={15} /> 文字
            </Link>
            <Link className="inline-flex h-9 items-center gap-2 rounded-full px-4 hover:text-neutral-950" to="/create/video">
              <Video size={15} /> 视频
            </Link>
          </nav>
        </header>

        <main className="grid flex-1 place-items-center py-10">
          <div className="w-full max-w-[440px] rounded-[28px] border border-neutral-200 bg-white p-6 shadow-[0_24px_80px_rgba(15,23,42,.08)]">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
