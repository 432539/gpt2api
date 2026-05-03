import { useQuery } from '@tanstack/react-query';
import { Activity, Cpu, KeyRound, Wallet } from 'lucide-react';

import { accountsApi } from '../../lib/services';
import { fmtNumber } from '../../lib/format';

export default function DashboardPage() {
  const { data: stats, isLoading } = useQuery({
    queryKey: ['admin', 'pool', 'stats'],
    queryFn: () => accountsApi.stats(),
    refetchInterval: 10_000,
  });

  const { data: gpt } = useQuery({
    queryKey: ['admin', 'accounts', 'list', 'gpt-summary'],
    queryFn: () => accountsApi.list({ provider: 'gpt', page_size: 1 }),
  });
  const { data: grok } = useQuery({
    queryKey: ['admin', 'accounts', 'list', 'grok-summary'],
    queryFn: () => accountsApi.list({ provider: 'grok', page_size: 1 }),
  });

  const gptAvail = stats?.pool?.gpt ?? 0;
  const grokAvail = stats?.pool?.grok ?? 0;
  const gptTotal = gpt?.total ?? 0;
  const grokTotal = grok?.total ?? 0;

  const KPIS = [
    {
      label: 'GPT 池可用',
      value: `${fmtNumber(gptAvail)} / ${fmtNumber(gptTotal)}`,
      delta: gptTotal > 0 ? `${Math.round((gptAvail / gptTotal) * 100)}%` : '—',
      icon: Cpu,
      highlight: true,
    },
    {
      label: 'GROK 池可用',
      value: `${fmtNumber(grokAvail)} / ${fmtNumber(grokTotal)}`,
      delta: grokTotal > 0 ? `${Math.round((grokAvail / grokTotal) * 100)}%` : '—',
      icon: Cpu,
    },
    { label: 'API Key', value: '—', delta: '即将上线', icon: KeyRound },
    { label: '今日流水', value: '—', delta: '即将上线', icon: Wallet },
  ];

  const POOLS = [
    { name: 'GPT 主池', active: gptAvail, total: gptTotal },
    { name: 'GROK 主池', active: grokAvail, total: grokTotal },
  ];

  return (
    <div className="page page-wide space-y-6">
      <header className="page-header">
        <div>
          <h1 className="page-title">概览</h1>
          <p className="page-subtitle">实时账号池与生成趋势。</p>
        </div>
      </header>

      <div className="stat-grid">
        {KPIS.map((k) => (
          <div key={k.label} className={`stat-tile ${k.highlight ? 'stat-tile-accent' : ''}`}>
            <p className="stat-label">
              {k.label}
              <k.icon size={16} className="opacity-80" />
            </p>
            <p className="stat-value">{isLoading ? '…' : k.value}</p>
            <p className="stat-delta">{k.delta}</p>
          </div>
        ))}
      </div>

      <div className="grid lg:grid-cols-[2fr_1fr] gap-4">
        <section className="card card-section">
          <h3 className="section-title mb-4">
            <Activity size={18} className="text-klein-500" />
            最近 24h 趋势
          </h3>
          <div className="h-64 grid place-items-center bg-klein-gradient-soft rounded-md text-text-tertiary text-small">
            （图表占位 · Sprint 10 接入 ECharts / Recharts）
          </div>
        </section>

        <section className="card card-section">
          <h3 className="section-title mb-4">账号池状态</h3>
          <div className="space-y-3">
            {POOLS.map((p) => {
              const ratio = p.total > 0 ? (p.active / p.total) * 100 : 0;
              const tone =
                ratio === 0
                  ? 'badge badge-danger'
                  : ratio < 50
                    ? 'badge badge-warning'
                    : 'badge badge-success';
              return (
                <div key={p.name} className="rounded-md border border-border p-3 bg-surface-1">
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-medium text-body">{p.name}</span>
                    <span className={tone}>
                      {p.total === 0 ? '空' : ratio === 0 ? '不可用' : '正常'}
                    </span>
                  </div>
                  <div className="progress">
                    <div className="progress-bar" style={{ width: `${ratio}%` }} />
                  </div>
                  <p className="text-small text-text-tertiary mt-1.5">
                    可用 {p.active} / 总 {p.total}
                  </p>
                </div>
              );
            })}
          </div>
        </section>
      </div>
    </div>
  );
}
