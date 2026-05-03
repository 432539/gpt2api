import { Copy, Share2, Users } from 'lucide-react';

import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

export default function InvitePage() {
  const me = useAuthStore((s) => s.me);
  const code = me?.invite_code ?? '—';
  const link =
    typeof window !== 'undefined'
      ? `${window.location.origin}/register?invite=${code}`
      : `https://gpt2api.example/register?invite=${code}`;

  const copy = (text: string, label: string) => {
    navigator.clipboard.writeText(text).then(() => toast.success(`${label}已复制`));
  };

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1 className="page-title">邀请中心</h1>
          <p className="page-subtitle">邀请好友注册即可获得专属奖励，更多返佣规则即将公布。</p>
        </div>
      </header>

      <section className="card-tinted card-section grid lg:grid-cols-[1fr_auto] gap-6 items-end mb-4">
        <div className="space-y-4 min-w-0">
          <div>
            <p className="text-overline mb-1">你的专属邀请码</p>
            <p className="font-mono text-display gradient-text break-all leading-tight">{code}</p>
          </div>
          <div>
            <p className="text-overline mb-1">邀请链接</p>
            <code className="block rounded-md bg-surface-1 border border-border px-4 py-2.5 font-mono text-small break-all">
              {link}
            </code>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-primary btn-lg" onClick={() => copy(code, '邀请码')}>
            <Copy size={16} /> 复制邀请码
          </button>
          <button className="btn btn-outline btn-lg" onClick={() => copy(link, '邀请链接')}>
            <Share2 size={16} /> 复制链接
          </button>
        </div>
      </section>

      <section className="card card-section">
        <h3 className="section-title mb-3">
          <Users size={16} className="text-text-tertiary" />
          规则说明
        </h3>
        <ul className="space-y-2 text-body text-text-secondary list-disc pl-5 leading-loose">
          <li>好友通过你的邀请码注册即视为关联，关系绑定后不可更改。</li>
          <li>好友首次充值时你会获得点数返利，具体比例由后台动态配置。</li>
          <li>系统会定期发放邀请奖励到你的钱包，可在「余额明细」中查看。</li>
          <li>禁止刷邀请、买卖邀请码等违规行为，违者扣除全部奖励。</li>
        </ul>
      </section>
    </div>
  );
}
