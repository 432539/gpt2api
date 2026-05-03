import { useQuery } from '@tanstack/react-query';
import {
  Activity,
  BarChart3,
  Coins,
  Image,
  KeyRound,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Users,
  Video,
} from 'lucide-react';
import type { ReactNode } from 'react';

import { dashboardApi } from '../../lib/services';
import type { DashboardProviderRow, DashboardRecentTask, DashboardTrendPoint } from '../../lib/types';
import { fmtNumber, fmtPoints, fmtTime } from '../../lib/format';

export default function DashboardPage() {
  const { data, isFetching, isLoading, refetch } = useQuery({
    queryKey: ['admin', 'dashboard', 'overview'],
    queryFn: () => dashboardApi.overview(),
    refetchInterval: 15_000,
  });

  const providers = data?.account_providers ?? [];
  const totalAccounts = providers.reduce((sum, row) => sum + row.total, 0);
  const availableAccounts = providers.reduce((sum, row) => sum + row.available, 0);
  const quotaRemaining = providers.reduce((sum, row) => sum + row.quota_remaining, 0);
  const quotaTotal = providers.reduce((sum, row) => sum + row.quota_total, 0);
  const quotaUsed = Math.max(0, quotaTotal - quotaRemaining);

  return (
    <div className="page page-wide space-y-5">
      <header className="page-header">
        <div>
          <h1 className="page-title">运营仪表盘</h1>
          <p className="page-subtitle">生成量、账号池、额度、用户与积分消耗的实时概览。</p>
        </div>
        <button className="btn btn-outline btn-md" onClick={() => refetch()} disabled={isFetching}>
          <RefreshCw size={16} className={isFetching ? 'animate-spin' : ''} /> 刷新
        </button>
      </header>

      <section className="grid gap-4 xl:grid-cols-[1.25fr_0.75fr]">
        <div className="relative overflow-hidden rounded-lg border border-border bg-klein-gradient p-6 text-white shadow-3">
          <div className="relative z-10 grid gap-6 lg:grid-cols-[1fr_auto] lg:items-end">
            <div>
              <div className="text-small opacity-80">今日生成任务</div>
              <div className="mt-2 text-[56px] leading-none font-bold tracking-normal tabular-nums">{isLoading ? '-' : fmtNumber(data?.generated_today)}</div>
              <div className="mt-3 flex flex-wrap gap-2 text-small">
                <span className="rounded-md bg-white/14 px-2.5 py-1">累计 {fmtNumber(data?.generated_total)}</span>
                <span className="rounded-md bg-white/14 px-2.5 py-1">成功率 {percent(data?.success_rate_today)}</span>
                <span className="rounded-md bg-white/14 px-2.5 py-1">今日用户 {fmtNumber(data?.active_users_today)}</span>
              </div>
            </div>
            <div className="grid grid-cols-3 gap-3 min-w-[420px]">
              <HeroMetric label="图片" value={fmtNumber(data?.image_today)} sub={`总 ${fmtNumber(data?.image_total)}`} icon={<Image size={20} />} />
              <HeroMetric label="视频" value={fmtNumber(data?.video_today)} sub={`总 ${fmtNumber(data?.video_total)}`} icon={<Video size={20} />} />
              <HeroMetric label="Token" value={compact(data?.text_tokens_today)} sub={`总 ${compact(data?.text_tokens_total)}`} icon={<Sparkles size={20} />} />
            </div>
          </div>
          <TrendChart points={data?.trend ?? []} />
        </div>

        <div className="grid gap-3 sm:grid-cols-3 xl:grid-cols-1">
          <SmallMetric title="账号可用" value={`${fmtNumber(availableAccounts)} / ${fmtNumber(totalAccounts)}`} icon={<KeyRound size={18} />} />
          <SmallMetric title="剩余额度" value={fmtNumber(quotaRemaining)} sub={quotaTotal > 0 ? `已用 ${fmtNumber(quotaUsed)} / ${fmtNumber(quotaTotal)}` : '等待探测额度'} icon={<ShieldCheck size={18} />} />
          <SmallMetric title="今日消耗" value={fmtPoints(data?.wallet_spend_today)} sub={`累计 ${fmtPoints(data?.wallet_spend_total)}`} icon={<Coins size={18} />} />
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-4">
        <InfoCard title="用户总数" value={fmtNumber(data?.users_total)} sub={`今日新增 ${fmtNumber(data?.users_today)}`} icon={<Users size={18} />} />
        <InfoCard title="任务积分" value={fmtPoints(data?.cost_points_today)} sub={`累计 ${fmtPoints(data?.cost_points_total)}`} icon={<Coins size={18} />} />
        <InfoCard title="图片产出" value={fmtNumber(data?.image_today)} sub={`累计 ${fmtNumber(data?.image_total)} 张`} icon={<Image size={18} />} />
        <InfoCard title="视频产出" value={fmtNumber(data?.video_today)} sub={`累计 ${fmtNumber(data?.video_total)} 个`} icon={<Video size={18} />} />
      </section>

      <section className="grid gap-4 xl:grid-cols-[1fr_1fr]">
        <div className="card card-section space-y-4">
          <header className="flex items-center justify-between gap-3">
            <h2 className="section-title"><BarChart3 size={18} className="text-klein-500" />账号池与额度</h2>
            <span className="badge badge-outline">每 15 秒刷新</span>
          </header>
          <div className="space-y-3">
            {providers.map((row) => <ProviderPanel key={row.provider} row={row} />)}
            {providers.length === 0 && <div className="text-small text-text-tertiary py-8 text-center">暂无账号池数据</div>}
          </div>
        </div>

        <div className="card card-section space-y-4">
          <header className="flex items-center justify-between gap-3">
            <h2 className="section-title"><Activity size={18} className="text-klein-500" />最近生成</h2>
            <span className="text-small text-text-tertiary">最新 8 条</span>
          </header>
          <div className="space-y-2">
            {(data?.recent_generations ?? []).map((row) => <RecentTask key={row.task_id} row={row} />)}
            {(data?.recent_generations ?? []).length === 0 && <div className="text-small text-text-tertiary py-8 text-center">暂无生成记录</div>}
          </div>
        </div>
      </section>
    </div>
  );
}

function HeroMetric({ label, value, sub, icon }: { label: string; value: string; sub: string; icon: ReactNode }) {
  return (
    <div className="rounded-md bg-white/12 p-4 backdrop-blur">
      <div className="flex items-center justify-between text-white/75 text-small">{label}{icon}</div>
      <div className="mt-3 text-[28px] leading-none font-semibold tabular-nums">{value}</div>
      <div className="mt-2 text-tiny text-white/70">{sub}</div>
    </div>
  );
}

function TrendChart({ points }: { points: DashboardTrendPoint[] }) {
  const rows = points.length > 0 ? points : Array.from({ length: 7 }, (_, i) => ({ date: `D${i + 1}`, generated: 0, cost_points: 0 }));
  const maxGenerated = Math.max(1, ...rows.map((p) => p.generated));
  const maxCost = Math.max(1, ...rows.map((p) => p.cost_points));
  const width = 960;
  const height = 150;
  const padX = 18;
  const padY = 18;
  const step = (width - padX * 2) / Math.max(1, rows.length - 1);
  const y = (v: number, max: number) => height - padY - (v / max) * (height - padY * 2);
  const line = (key: 'generated' | 'cost_points', max: number) =>
    rows.map((p, i) => `${padX + i * step},${y(p[key], max)}`).join(' ');
  const area = `${padX},${height - padY} ${line('generated', maxGenerated)} ${width - padX},${height - padY}`;

  return (
    <div className="relative z-10 mt-8 rounded-md border border-white/15 bg-white/8 p-4">
      <div className="mb-2 flex items-center justify-between gap-3 text-small">
        <div className="font-medium text-white/90">近 7 天趋势</div>
        <div className="flex items-center gap-3 text-tiny text-white/70">
          <span className="inline-flex items-center gap-1"><i className="h-2 w-2 rounded-full bg-white" />生成量</span>
          <span className="inline-flex items-center gap-1"><i className="h-2 w-2 rounded-full bg-cyan-200" />消耗积分</span>
        </div>
      </div>
      <svg viewBox={`0 0 ${width} ${height}`} className="h-[150px] w-full overflow-visible">
        {[0, 1, 2].map((i) => (
          <line key={i} x1={padX} x2={width - padX} y1={padY + i * 52} y2={padY + i * 52} stroke="rgba(255,255,255,.14)" strokeWidth="1" />
        ))}
        <polygon points={area} fill="rgba(255,255,255,.10)" />
        <polyline points={line('generated', maxGenerated)} fill="none" stroke="rgba(255,255,255,.96)" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" />
        <polyline points={line('cost_points', maxCost)} fill="none" stroke="rgb(165,243,252)" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
        {rows.map((p, i) => (
          <g key={p.date}>
            <circle cx={padX + i * step} cy={y(p.generated, maxGenerated)} r="4" fill="white" />
            <text x={padX + i * step} y={height - 2} textAnchor="middle" fontSize="18" fill="rgba(255,255,255,.70)">
              {formatDay(p.date)}
            </text>
          </g>
        ))}
      </svg>
    </div>
  );
}

function SmallMetric({ title, value, sub, icon }: { title: string; value: string; sub?: string; icon: ReactNode }) {
  return (
    <div className="card card-section !py-4">
      <div className="flex items-center justify-between gap-3">
        <div className="text-small text-text-tertiary">{title}</div>
        <span className="grid h-8 w-8 place-items-center rounded-md bg-info-soft text-klein-500">{icon}</span>
      </div>
      <div className="mt-2 text-h3 font-semibold text-text-primary tabular-nums">{value}</div>
      {sub && <div className="mt-1 text-tiny text-text-tertiary">{sub}</div>}
    </div>
  );
}

function InfoCard({ title, value, sub, icon }: { title: string; value: string; sub: string; icon: ReactNode }) {
  return (
    <div className="card card-section !py-4">
      <div className="flex items-center gap-3">
        <span className="grid h-9 w-9 place-items-center rounded-md bg-info-soft text-klein-500">{icon}</span>
        <div>
          <div className="text-small text-text-tertiary">{title}</div>
          <div className="mt-1 text-[24px] font-semibold text-text-primary tabular-nums">{value}</div>
        </div>
      </div>
      <div className="mt-3 text-tiny text-text-tertiary">{sub}</div>
    </div>
  );
}

function ProviderPanel({ row }: { row: DashboardProviderRow }) {
  const availableRatio = row.total > 0 ? row.available / row.total : 0;
  const quotaRatio = row.quota_total > 0 ? row.quota_remaining / row.quota_total : 0;
  return (
    <div className="rounded-md border border-border bg-surface-1 p-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="text-base font-semibold text-text-primary uppercase">{row.provider}</div>
          <div className="text-tiny text-text-tertiary mt-0.5">OK {fmtNumber(row.test_ok)} · 熔断 {fmtNumber(row.broken)} · 成功 {fmtNumber(row.success_count)} · 错误 {fmtNumber(row.error_count)}</div>
        </div>
        <span className={availableRatio === 0 ? 'badge badge-danger' : availableRatio < 0.5 ? 'badge badge-warning' : 'badge badge-success'}>
          可用 {fmtNumber(row.available)} / {fmtNumber(row.total)}
        </span>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <Progress label="账号可用率" value={availableRatio} text={`${Math.round(availableRatio * 100)}%`} />
        <Progress label="额度剩余率" value={quotaRatio} text={row.quota_total > 0 ? `${fmtNumber(row.quota_remaining)} / ${fmtNumber(row.quota_total)}` : '未探测'} />
      </div>
    </div>
  );
}

function Progress({ label, value, text }: { label: string; value: number; text: string }) {
  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between text-tiny text-text-tertiary">
        <span>{label}</span>
        <span>{text}</span>
      </div>
      <div className="progress">
        <div className="progress-bar" style={{ width: `${Math.max(0, Math.min(100, value * 100))}%` }} />
      </div>
    </div>
  );
}

function RecentTask({ row }: { row: DashboardRecentTask }) {
  return (
    <div className="grid gap-3 rounded-md border border-border bg-surface-1 p-3 md:grid-cols-[1fr_auto] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium text-text-primary">{row.user_label}</span>
          <span className="badge badge-outline">{row.kind === 'video' ? '视频' : '图片'} x{row.count}</span>
          <span className={statusClass(row.status)}>{statusText(row.status)}</span>
        </div>
        <div className="mt-1 text-tiny text-text-tertiary truncate">{row.model_code} · {row.task_id} · {fmtTime(row.created_at)}</div>
      </div>
      <div className="text-right text-small font-semibold text-text-primary tabular-nums">{fmtPoints(row.cost_points)} 点</div>
    </div>
  );
}

function statusText(status: number) {
  if (status === 2) return '成功';
  if (status === 3) return '失败';
  if (status === 4) return '已退款';
  if (status === 1) return '运行中';
  return '排队中';
}

function statusClass(status: number) {
  if (status === 2) return 'badge badge-success';
  if (status === 3 || status === 4) return 'badge badge-danger';
  if (status === 1) return 'badge badge-warning';
  return 'badge badge-outline';
}

function percent(v?: number) {
  if (v == null) return '-';
  return `${Math.round(v * 100)}%`;
}

function compact(v?: number | null) {
  const n = Number(v || 0);
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${(n / 1000).toFixed(0)}K`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
  return fmtNumber(n);
}

function formatDay(v: string) {
  if (!v.includes('-')) return v;
  return v.slice(5).replace('-', '/');
}
