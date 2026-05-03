import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CheckCircle2, CircleOff, Copy, Plus, RefreshCw, Save, Trash2, WalletCards } from 'lucide-react';
import { useEffect, useMemo, useState, type ReactNode } from 'react';

import { ApiError } from '../../lib/api';
import { systemApi } from '../../lib/services';
import type { SystemSettings } from '../../lib/types';
import { toast } from '../../stores/toast';

interface RechargePackage {
  id: string;
  name: string;
  amount: number;
  points: number;
  bonus_points: number;
  enabled: boolean;
  sort_order: number;
  badge: string;
  remark: string;
}

const DEFAULT_ROWS: RechargePackage[] = [
  { id: 'p100', name: '100 点套餐', amount: 10, points: 100, bonus_points: 0, enabled: true, sort_order: 10, badge: '', remark: '' },
  { id: 'p500', name: '500 点套餐', amount: 45, points: 500, bonus_points: 50, enabled: true, sort_order: 20, badge: '推荐', remark: '' },
];

const asNum = (v: unknown, fallback: number) => {
  const n = Number(v);
  return Number.isFinite(n) ? n : fallback;
};
const asBool = (v: unknown, fallback = false) => (v == null ? fallback : Boolean(v));

function fromValue(v: unknown): RechargePackage[] {
  if (!Array.isArray(v)) return DEFAULT_ROWS;
  return v.map((item, idx) => {
    const row = item as Partial<RechargePackage>;
    return {
      id: String(row.id || `pkg_${idx + 1}`),
      name: String(row.name || ''),
      amount: asNum(row.amount, 0),
      points: asNum(row.points, 0) / 100,
      bonus_points: asNum(row.bonus_points, 0) / 100,
      enabled: asBool(row.enabled, true),
      sort_order: asNum(row.sort_order, (idx + 1) * 10),
      badge: String(row.badge || ''),
      remark: String(row.remark || ''),
    };
  });
}

function toPayload(rows: RechargePackage[]): Partial<SystemSettings> {
  return {
    'recharge.packages': rows.map((row) => ({
      id: row.id.trim(),
      name: row.name.trim(),
      amount: Number(row.amount) || 0,
      points: Math.round((Number(row.points) || 0) * 100),
      bonus_points: Math.round((Number(row.bonus_points) || 0) * 100),
      enabled: row.enabled,
      sort_order: Number(row.sort_order) || 0,
      badge: row.badge.trim(),
      remark: row.remark.trim(),
    })),
  };
}

export default function RechargePackagesPage() {
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ['admin', 'system', 'settings'], queryFn: () => systemApi.get() });
  const [rows, setRows] = useState<RechargePackage[]>(DEFAULT_ROWS);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (settings.data) {
      setRows(fromValue(settings.data['recharge.packages']));
      setDirty(false);
    }
  }, [settings.data]);

  const totals = useMemo(() => {
    const enabled = rows.filter((row) => row.enabled);
    return {
      total: rows.length,
      enabled: enabled.length,
      disabled: rows.length - enabled.length,
    };
  }, [rows]);

  const update = (idx: number, patch: Partial<RechargePackage>) => {
    setRows((old) => old.map((row, i) => (i === idx ? { ...row, ...patch } : row)));
    setDirty(true);
  };

  const addRow = () => {
    setRows((old) => [
      ...old,
      {
        id: `pkg_${Date.now()}`,
        name: '',
        amount: 0,
        points: 0,
        bonus_points: 0,
        enabled: true,
        sort_order: (old.length + 1) * 10,
        badge: '',
        remark: '',
      },
    ]);
    setDirty(true);
  };

  const cloneRow = (idx: number) => {
    const row = rows[idx];
    if (!row) return;
    setRows((old) => [
      ...old.slice(0, idx + 1),
      { ...row, id: `${row.id}_copy`, name: `${row.name} 副本`, sort_order: row.sort_order + 1 },
      ...old.slice(idx + 1),
    ]);
    setDirty(true);
  };

  const save = useMutation({
    mutationFn: () => systemApi.update(toPayload(rows)),
    onSuccess: () => {
      toast.success('充值套餐已保存');
      setDirty(false);
      qc.invalidateQueries({ queryKey: ['admin', 'system'] });
    },
    onError: (e: ApiError | Error) => toast.error(e.message),
  });

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">充值套餐</h1>
          <p className="page-subtitle">用表单维护前端售卖套餐，金额单位为元，积分单位为点。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-outline btn-md" onClick={() => settings.refetch()} disabled={settings.isFetching}>
            <RefreshCw size={16} className={settings.isFetching ? 'animate-spin' : ''} /> 重新加载
          </button>
          <button className="btn btn-outline btn-md" onClick={addRow}>
            <Plus size={16} /> 新增套餐
          </button>
          <button className="btn btn-primary btn-md" onClick={() => save.mutate()} disabled={!dirty || save.isPending}>
            <Save size={16} /> {save.isPending ? '保存中...' : dirty ? '保存修改' : '已是最新'}
          </button>
        </div>
      </header>

      <div className="grid gap-3 md:grid-cols-3">
        <Stat title="套餐总数" value={totals.total} icon={<WalletCards size={18} />} />
        <Stat title="已启用" value={totals.enabled} icon={<CheckCircle2 size={18} />} />
        <Stat title="已停用" value={totals.disabled} icon={<CircleOff size={18} />} />
      </div>

      {settings.isLoading ? (
        <div className="card card-section text-center text-text-tertiary py-10">加载中...</div>
      ) : (
        <div className="card table-wrap">
          <table className="data-table min-w-[1180px]">
            <thead>
              <tr>
                <th>排序</th>
                <th>套餐 ID</th>
                <th>套餐名称</th>
                <th>金额（元）</th>
                <th>基础积分（点）</th>
                <th>赠送积分（点）</th>
                <th>标签</th>
                <th>备注</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row, idx) => (
                <tr key={`${row.id}-${idx}`}>
                  <td><input className="input w-[76px] tabular-nums" type="number" value={row.sort_order} onChange={(e) => update(idx, { sort_order: Number(e.target.value) || 0 })} /></td>
                  <td><input className="input min-w-[140px] font-mono" value={row.id} onChange={(e) => update(idx, { id: e.target.value })} placeholder="p100" /></td>
                  <td><input className="input min-w-[160px]" value={row.name} onChange={(e) => update(idx, { name: e.target.value })} placeholder="100 点套餐" /></td>
                  <td><input className="input w-[110px] tabular-nums" type="number" min={0} step="0.01" value={row.amount} onChange={(e) => update(idx, { amount: Number(e.target.value) || 0 })} /></td>
                  <td><input className="input w-[120px] tabular-nums" type="number" min={0} value={row.points} onChange={(e) => update(idx, { points: Number(e.target.value) || 0 })} /></td>
                  <td><input className="input w-[120px] tabular-nums" type="number" min={0} value={row.bonus_points} onChange={(e) => update(idx, { bonus_points: Number(e.target.value) || 0 })} /></td>
                  <td><input className="input min-w-[100px]" value={row.badge} onChange={(e) => update(idx, { badge: e.target.value })} placeholder="推荐" /></td>
                  <td><input className="input min-w-[180px]" value={row.remark} onChange={(e) => update(idx, { remark: e.target.value })} placeholder="内部备注" /></td>
                  <td>
                    <button className={row.enabled ? 'btn btn-outline btn-sm' : 'btn btn-ghost btn-sm'} onClick={() => update(idx, { enabled: !row.enabled })}>
                      {row.enabled ? '启用' : '停用'}
                    </button>
                  </td>
                  <td>
                    <div className="flex items-center gap-1">
                      <button className="btn btn-ghost btn-icon btn-sm" title="复制套餐" onClick={() => cloneRow(idx)}><Copy size={14} /></button>
                      <button className="btn btn-danger-ghost btn-icon btn-sm" title="删除套餐" onClick={() => { setRows((old) => old.filter((_, i) => i !== idx)); setDirty(true); }}><Trash2 size={14} /></button>
                    </div>
                  </td>
                </tr>
              ))}
              {rows.length === 0 && (
                <tr>
                  <td colSpan={10} className="text-center text-text-tertiary py-10">暂无套餐，点击右上角新增。</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function Stat({ title, value, icon }: { title: string; value: number; icon: ReactNode }) {
  return (
    <section className="card card-section flex items-center justify-between">
      <div>
        <div className="text-small text-text-tertiary">{title}</div>
        <div className="text-[26px] font-semibold text-text-primary tabular-nums mt-1">{value}</div>
      </div>
      <span className="grid place-items-center w-10 h-10 rounded-md bg-info-soft text-klein-500">{icon}</span>
    </section>
  );
}
