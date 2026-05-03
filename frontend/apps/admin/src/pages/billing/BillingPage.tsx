import { useQuery } from '@tanstack/react-query';
import { RefreshCw, Search, Wallet } from 'lucide-react';
import { useMemo, useState } from 'react';

import { billingApi } from '../../lib/services';
import type { AdminWalletLogItem } from '../../lib/types';
import { fmtNumber, fmtPoints, fmtTime } from '../../lib/format';

const BIZ_OPTIONS = [
  { value: '', label: '全部业务' },
  { value: 'recharge', label: '充值' },
  { value: 'consume', label: '消费' },
  { value: 'refund', label: '退款' },
  { value: 'cdk', label: '兑换码' },
  { value: 'promo', label: '优惠码' },
  { value: 'invite_reward', label: '邀请奖励' },
  { value: 'gift', label: '赠送' },
];

export default function BillingPage() {
  const [keyword, setKeyword] = useState('');
  const [userID, setUserID] = useState('');
  const [bizType, setBizType] = useState('');
  const [direction, setDirection] = useState<'' | '1' | '-1'>('');
  const [page, setPage] = useState(1);
  const pageSize = 20;

  const query = useQuery({
    queryKey: ['admin', 'billing', 'wallet-logs', keyword, userID, bizType, direction, page],
    queryFn: () => billingApi.walletLogs({
      keyword: keyword.trim() || undefined,
      user_id: Number(userID) || undefined,
      biz_type: bizType || undefined,
      direction: direction ? Number(direction) as 1 | -1 : '',
      page,
      page_size: pageSize,
    }),
  });

  const rows = query.data?.list ?? [];
  const total = query.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const summary = useMemo(() => {
    let income = 0;
    let outcome = 0;
    for (const row of rows) {
      if (row.direction > 0) income += row.points;
      if (row.direction < 0) outcome += Math.abs(row.points);
    }
    return { income, outcome };
  }, [rows]);

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title flex items-center gap-2"><Wallet className="text-klein-500" size={26} />充值消费记录</h1>
          <p className="page-subtitle">查看用户积分流水，包含充值、消费、退款、兑换码、优惠码和人工调整。</p>
        </div>
        <button className="btn btn-outline btn-md" onClick={() => query.refetch()} disabled={query.isFetching}>
          <RefreshCw size={16} className={query.isFetching ? 'animate-spin' : ''} /> 刷新
        </button>
      </header>

      <div className="grid gap-3 md:grid-cols-3">
        <Stat title="当前页收入" value={fmtPoints(summary.income)} />
        <Stat title="当前页支出" value={fmtPoints(summary.outcome)} />
        <Stat title="匹配记录" value={fmtNumber(total)} />
      </div>

      <div className="card card-section grid gap-2 !py-3 lg:grid-cols-[minmax(320px,1fr)_140px_140px_120px]">
        <div className="relative min-w-0">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input
            className="input pl-9"
            value={keyword}
            onChange={(e) => { setKeyword(e.target.value); setPage(1); }}
            placeholder="搜索流水ID、用户、业务ID、备注"
          />
        </div>
        <input
          className="input w-full"
          value={userID}
          onChange={(e) => { setUserID(e.target.value); setPage(1); }}
          placeholder="用户ID"
        />
        <select className="select select-sm w-full" value={bizType} onChange={(e) => { setBizType(e.target.value); setPage(1); }}>
          {BIZ_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <select className="select select-sm w-full" value={direction} onChange={(e) => { setDirection(e.target.value as typeof direction); setPage(1); }}>
          <option value="">收支方向</option>
          <option value="1">收入</option>
          <option value="-1">支出</option>
        </select>
      </div>

      <div className="card table-wrap">
        <table className="data-table min-w-[1120px]">
          <thead>
            <tr>
              <th>时间</th>
              <th>用户</th>
              <th>业务</th>
              <th>业务ID</th>
              <th>方向</th>
              <th>变动积分</th>
              <th>变动前</th>
              <th>变动后</th>
              <th>备注</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => <LogRow key={row.id} row={row} />)}
            {!query.isLoading && rows.length === 0 && (
              <tr><td colSpan={9} className="py-10 text-center text-text-tertiary">暂无记录</td></tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="card card-section flex flex-wrap items-center justify-between gap-3 !py-2">
        <span className="text-small text-text-tertiary">第 {page} / {pages} 页，共 {fmtNumber(total)} 条</span>
        <div className="flex gap-2">
          <button className="btn btn-outline btn-sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</button>
          <button className="btn btn-outline btn-sm" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>下一页</button>
        </div>
      </div>
    </div>
  );
}

function LogRow({ row }: { row: AdminWalletLogItem }) {
  const isIncome = row.direction > 0;
  return (
    <tr>
      <td className="whitespace-nowrap">{fmtTime(row.created_at)}</td>
      <td>
        <div className="font-medium text-text-primary">{row.user_label || `用户 ${row.user_id}`}</div>
        <div className="text-tiny text-text-tertiary">ID {row.user_id}</div>
      </td>
      <td><span className="badge badge-outline">{bizLabel(row.biz_type)}</span></td>
      <td className="font-mono text-small max-w-[220px] truncate" title={row.biz_id}>{row.biz_id}</td>
      <td><span className={isIncome ? 'badge badge-success' : 'badge badge-danger'}>{isIncome ? '收入' : '支出'}</span></td>
      <td className={isIncome ? 'text-success font-semibold tabular-nums' : 'text-danger font-semibold tabular-nums'}>
        {isIncome ? '+' : '-'}{fmtPoints(Math.abs(row.points))}
      </td>
      <td className="tabular-nums">{fmtPoints(row.points_before)}</td>
      <td className="tabular-nums">{fmtPoints(row.points_after)}</td>
      <td className="max-w-[240px] truncate" title={row.remark}>{row.remark || '-'}</td>
    </tr>
  );
}

function Stat({ title, value }: { title: string; value: string }) {
  return (
    <section className="card card-section !py-3">
      <div className="text-small text-text-tertiary">{title}</div>
      <div className="mt-1 text-h3 font-semibold text-text-primary tabular-nums">{value}</div>
    </section>
  );
}

function bizLabel(v: string) {
  return BIZ_OPTIONS.find((o) => o.value === v)?.label || v || '-';
}
