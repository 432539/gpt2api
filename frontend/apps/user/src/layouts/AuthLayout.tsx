import { Outlet } from 'react-router-dom';
import { Logo } from '../components/Logo';

export function AuthLayout() {
  return (
    <div
      className="
        relative grid min-h-full
        lg:grid-cols-[1.1fr_1fr]
        bg-surface-bg
      "
    >
      {/* 左侧：品牌叙事（移动端隐藏） */}
      <aside className="relative hidden overflow-hidden lg:block">
        <div className="absolute inset-0 bg-klein-gradient" />
        <div className="absolute inset-0 opacity-20"
             style={{
               backgroundImage:
                 'radial-gradient(circle at 20% 20%, rgba(255,255,255,.6) 0, transparent 50%), radial-gradient(circle at 80% 60%, rgba(255,255,255,.4) 0, transparent 55%)',
             }}
        />
        <div className="relative z-10 flex h-full flex-col justify-between p-12 text-text-on-klein">
          <Logo size="lg" />
          <div className="space-y-6 max-w-lg">
            <h1 className="text-display leading-tight">
              一句话生图<br />一张图成片
            </h1>
            <p className="text-body opacity-85 leading-loose">
              基于 GPT 与 GROK 双账号池的高并发 AIGC 平台，<br />
              覆盖手机 / 平板 / 桌面 / 4K 大屏，全 OpenAI 协议兼容。
            </p>
            <ul className="space-y-3 text-body opacity-90">
              <li>· 多模型并行调度，告别排队</li>
              <li>· OpenAI 兼容协议，5 分钟接入</li>
              <li>· 邀请返点 / 套餐 / CDK 灵活计费</li>
            </ul>
          </div>
          <p className="text-small opacity-70">© gpt2api · 多终端响应式 AIGC 平台</p>
        </div>
      </aside>

      {/* 右侧：表单插槽 */}
      <main className="relative grid place-items-center px-4 py-10 sm:px-8">
        <div className="w-full max-w-md klein-fade-in">
          <div className="lg:hidden mb-8 flex justify-center">
            <Logo size="lg" />
          </div>
          <Outlet />
        </div>
      </main>
    </div>
  );
}
