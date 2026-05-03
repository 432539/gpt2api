import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Gift, Sparkles, Wallet } from 'lucide-react';

import { ApiError } from '../../lib/api';
import { fmtBiz, fmtPoints, fmtTime, pointsClass } from '../../lib/format';
import { billingApi } from '../../lib/services';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

export default function BillingPage() {
  const me = useAuthStore((s) => s.me);
  const refreshMe = useAuthStore((s) => s.refreshMe);
  const qc = useQueryClient();

  const [page, setPage] = useState(1);
  const logsQ = useQuery({
    queryKey: ['billing.logs', page],
    queryFn: () => billingApi.logs(page, 20),
  });

  const [code, setCode] = useState('');
  const redeemMut = useMutation({
    mutationFn: () => billingApi.redeemCDK(code.trim()),
    onSuccess: async (resp) => {
      toast.success(`兑换成功 +${fmtPoints(resp.points)} 点`);
      setCode('');
      await refreshMe();
      await qc.invalidateQueries({ queryKey: ['billing.logs'] });
      setPage(1);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '兑换失败'),
  });

  const stats = [
    { label: '可用点数', value: fmtPoints(me?.points ?? 0), accent: true },
    { label: '冻结点数', value: fmtPoints(me?.frozen_points ?? 0) },
    { label: '当前套餐', value: me?.plan_code?.toUpperCase() ?? 'FREE' },
    { label: '邀请码', value: me?.invite_code ?? '—' },
  ];

  const logs = logsQ.data?.list ?? [];
  const total = logsQ.data?.total ?? 0;
  const pageSize = logsQ.data?.page_size ?? 20;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1 className="page-title">余额明细</h1>
          <p className="page-subtitle">点数变动、兑换码、充值套餐都在这里管理。</p>
        </div>
      </header>

      <div className="stat-grid mb-6">
        {stats.map((s) => (
          <div key={s.label} className={`stat-tile ${s.accent ? 'stat-tile-accent' : ''}`}>
            <p className="stat-label">{s.label}</p>
            <p className="stat-value">{s.value}</p>
          </div>
        ))}
      </div>

      <section className="grid gap-4 mb-6 lg:grid-cols-2">
        <div className="card card-section">
          <header className="section-header mb-3">
            <span className="section-title">
              <Gift size={18} className="text-klein-500" />
              兑换码 CDK
            </span>
          </header>
          <p className="text-small text-text-secondary mb-4 leading-loose">
            输入活动码或邀请码即可立刻到账点数；同一个兑换码不可重复使用。
          </p>
          <div className="flex flex-col sm:flex-row gap-2">
            <input
              className="input"
              placeholder="例如：GPT2API-2026-WELCOME"
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              maxLength={32}
            />
            <button
              className="btn btn-primary btn-lg whitespace-nowrap"
              disabled={code.trim().length < 4 || redeemMut.isPending}
              onClick={() => redeemMut.mutate()}
              type="button"
            >
              {redeemMut.isPending ? '兑换中…' : '立即兑换'}
            </button>
          </div>
        </div>

        <div className="card-tinted card-section">
          <header className="section-header mb-3">
            <span className="section-title">
              <Sparkles size={18} className="text-klein-500" />
              充值套餐
            </span>
            <span className="badge badge-klein">即将上线</span>
          </header>
          <p className="text-small text-text-secondary mb-4 leading-loose">
            支付通道（微信 / 支付宝 / Stripe）正在开发中，当前请通过 CDK 或邀请获得点数。
          </p>
          <button className="btn btn-outline btn-md" disabled type="button">
            敬请期待
          </button>
          <p className="mt-3 text-small text-text-tertiary leading-loose">
            冻结点数不会按时间自动释放，它只会在任务成功结算时转为已消费，或在任务失败、超时后自动退款解冻。
          </p>
        </div>
      </section>

      <section className="card overflow-hidden">
        <div className="px-5 py-3.5 border-b border-border flex items-center justify-between">
          <span className="section-title">
            <Wallet size={16} className="text-text-tertiary" />
            最近交易
          </span>
          <span className="text-small text-text-tertiary">共 {total} 条</span>
        </div>
        <div className="divide-y divide-border">
          {logsQ.isLoading && (
            <p className="px-5 py-10 text-center text-text-tertiary text-small">加载中...</p>
          )}
          {!logsQ.isLoading && logs.length === 0 && (
            <div className="empty-state">
              <span className="empty-state-icon">
                <Wallet size={22} />
              </span>
              <p className="empty-state-title">暂无流水记录</p>
              <p className="empty-state-desc">兑换 CDK、生成图片或视频后，相关账单会在此呈现。</p>
            </div>
          )}
          {logs.map((l) => (
            <div key={l.id} className="list-row">
              <div className="min-w-0">
                <p className="font-medium text-text-primary truncate">
                  {fmtBiz(l.biz_type)}
                  {l.remark ? ` · ${l.remark}` : ''}
                </p>
                <p className="text-small text-text-tertiary mt-0.5">{fmtTime(l.created_at)}</p>
              </div>
              <p className={`font-bold whitespace-nowrap ${pointsClass(l.direction)}`}>
                {l.direction > 0 ? '+' : '-'} {fmtPoints(Math.abs(l.points))} 点
              </p>
            </div>
          ))}
        </div>
        <div className="flex items-center justify-between gap-3 border-t border-border px-5 py-4 text-sm">
          <span className="text-text-tertiary">
            第 {page} / {totalPages} 页，共 {total} 条
          </span>
          <div className="flex items-center gap-2">
            <button
              className="btn btn-outline btn-md"
              disabled={page <= 1 || logsQ.isFetching}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              type="button"
            >
              上一页
            </button>
            <button
              className="btn btn-outline btn-md"
              disabled={page >= totalPages || logsQ.isFetching}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              type="button"
            >
              下一页
            </button>
          </div>
        </div>
      </section>
    </div>
  );
}
