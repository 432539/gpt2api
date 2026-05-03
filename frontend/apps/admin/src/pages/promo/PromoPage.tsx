import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Edit3, Plus, RefreshCw, Search, Tag, Trash2 } from 'lucide-react';
import { useState, type ReactNode } from 'react';

import { ApiError } from '../../lib/api';
import { promoApi } from '../../lib/services';
import type { AdminPromoBody, AdminPromoItem } from '../../lib/types';
import { fmtNumber, fmtPoints, fmtTime } from '../../lib/format';
import { toast } from '../../stores/toast';

interface FormState {
  id?: number;
  code: string;
  name: string;
  discount_type: 1 | 2 | 3;
  discount_val: number;
  min_amount: number;
  apply_to: string;
  total_qty: number;
  per_user_limit: number;
  start_at: string;
  end_at: string;
  status: 0 | 1;
}

const DEFAULT_FORM: FormState = {
  code: '',
  name: '',
  discount_type: 1,
  discount_val: 0,
  min_amount: 0,
  apply_to: 'all',
  total_qty: 0,
  per_user_limit: 1,
  start_at: '',
  end_at: '',
  status: 1,
};

export default function PromoPage() {
  const qc = useQueryClient();
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<'' | '0' | '1'>('');
  const [discountType, setDiscountType] = useState<'' | '1' | '2' | '3'>('');
  const [page, setPage] = useState(1);
  const [form, setForm] = useState<FormState | null>(null);
  const pageSize = 20;

  const query = useQuery({
    queryKey: ['admin', 'promo', keyword, status, discountType, page],
    queryFn: () => promoApi.list({
      keyword: keyword.trim() || undefined,
      status: status === '' ? '' : Number(status) as 0 | 1,
      discount_type: discountType === '' ? '' : Number(discountType) as 1 | 2 | 3,
      page,
      page_size: pageSize,
    }),
  });

  const rows = query.data?.list ?? [];
  const total = query.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));

  const save = useMutation({
    mutationFn: (f: FormState) => {
      const body = formToBody(f);
      return f.id ? promoApi.update(f.id, body) : promoApi.create(body).then(() => undefined);
    },
    onSuccess: () => {
      toast.success('优惠码已保存');
      setForm(null);
      qc.invalidateQueries({ queryKey: ['admin', 'promo'] });
    },
    onError: (e: ApiError | Error) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: number) => promoApi.remove(id),
    onSuccess: () => {
      toast.success('优惠码已删除');
      qc.invalidateQueries({ queryKey: ['admin', 'promo'] });
    },
    onError: (e: ApiError | Error) => toast.error(e.message),
  });

  const toggle = useMutation({
    mutationFn: (row: AdminPromoItem) => promoApi.update(row.id, { status: row.status === 1 ? 0 : 1 }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin', 'promo'] }),
    onError: (e: ApiError | Error) => toast.error(e.message),
  });

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title flex items-center gap-2"><Tag className="text-klein-500" size={26} />优惠码</h1>
          <p className="page-subtitle">创建和维护满减、折扣、赠点优惠码，支持总量、每用户次数、有效期和启停控制。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-outline btn-md" onClick={() => query.refetch()} disabled={query.isFetching}>
            <RefreshCw size={16} className={query.isFetching ? 'animate-spin' : ''} /> 刷新
          </button>
          <button className="btn btn-primary btn-md" onClick={() => setForm(DEFAULT_FORM)}>
            <Plus size={16} /> 新增优惠码
          </button>
        </div>
      </header>

      <div className="card card-section flex flex-wrap items-center gap-2 !py-3">
        <div className="relative min-w-[220px] flex-1">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input className="input pl-9" value={keyword} onChange={(e) => { setKeyword(e.target.value); setPage(1); }} placeholder="搜索优惠码、名称、ID" />
        </div>
        <select className="select select-sm min-w-[110px]" value={status} onChange={(e) => { setStatus(e.target.value as typeof status); setPage(1); }}>
          <option value="">全部状态</option>
          <option value="1">启用</option>
          <option value="0">停用</option>
        </select>
        <select className="select select-sm min-w-[120px]" value={discountType} onChange={(e) => { setDiscountType(e.target.value as typeof discountType); setPage(1); }}>
          <option value="">全部类型</option>
          <option value="1">满减</option>
          <option value="2">折扣</option>
          <option value="3">赠点</option>
        </select>
      </div>

      <div className="card table-wrap">
        <table className="data-table min-w-[1120px]">
          <thead>
            <tr>
              <th>优惠码</th>
              <th>类型</th>
              <th>优惠值</th>
              <th>门槛</th>
              <th>适用范围</th>
              <th>使用量</th>
              <th>每用户</th>
              <th>有效期</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.id}>
                <td>
                  <div className="font-semibold text-text-primary">{row.code}</div>
                  <div className="text-tiny text-text-tertiary">{row.name}</div>
                </td>
                <td><span className="badge badge-outline">{discountLabel(row.discount_type)}</span></td>
                <td className="font-semibold">{discountValue(row)}</td>
                <td>{row.min_amount > 0 ? `${fmtNumber(row.min_amount / 100)} 元` : '无门槛'}</td>
                <td>{row.apply_to || 'all'}</td>
                <td>{fmtNumber(row.used_qty)} / {row.total_qty > 0 ? fmtNumber(row.total_qty) : '不限'}</td>
                <td>{row.per_user_limit > 0 ? `${row.per_user_limit} 次` : '不限'}</td>
                <td className="whitespace-nowrap">{fmtTime(row.start_at)} - {fmtTime(row.end_at)}</td>
                <td><button className={row.status === 1 ? 'btn btn-outline btn-sm' : 'btn btn-ghost btn-sm'} onClick={() => toggle.mutate(row)}>{row.status === 1 ? '启用' : '停用'}</button></td>
                <td>
                  <div className="flex items-center gap-1">
                    <button className="btn btn-ghost btn-icon btn-sm" onClick={() => setForm(rowToForm(row))} title="编辑"><Edit3 size={14} /></button>
                    <button className="btn btn-danger-ghost btn-icon btn-sm" onClick={() => { if (confirm(`删除优惠码 ${row.code}？`)) remove.mutate(row.id); }} title="删除"><Trash2 size={14} /></button>
                  </div>
                </td>
              </tr>
            ))}
            {!query.isLoading && rows.length === 0 && (
              <tr><td colSpan={10} className="py-10 text-center text-text-tertiary">暂无优惠码</td></tr>
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

      {form && <PromoDialog form={form} setForm={setForm} saving={save.isPending} onClose={() => setForm(null)} onSave={() => save.mutate(form)} />}
    </div>
  );
}

function PromoDialog({ form, setForm, saving, onClose, onSave }: { form: FormState; setForm: (f: FormState | null) => void; saving: boolean; onClose: () => void; onSave: () => void }) {
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setForm({ ...form, [k]: v });
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-surface-overlay p-4">
      <div className="card card-section w-full max-w-3xl space-y-4">
        <header className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-h4 font-semibold text-text-primary">{form.id ? '编辑优惠码' : '新增优惠码'}</h2>
            <p className="text-small text-text-tertiary">金额单位为元，赠点单位为点。</p>
          </div>
          <button className="btn btn-ghost btn-sm" onClick={onClose}>关闭</button>
        </header>
        <div className="grid gap-3 md:grid-cols-2">
          <Field label="优惠码"><input className="input font-mono" value={form.code} onChange={(e) => set('code', e.target.value.toUpperCase())} placeholder="SPRING2026" /></Field>
          <Field label="名称"><input className="input" value={form.name} onChange={(e) => set('name', e.target.value)} placeholder="春季活动" /></Field>
          <Field label="类型">
            <select className="select" value={form.discount_type} onChange={(e) => set('discount_type', Number(e.target.value) as 1 | 2 | 3)}>
              <option value={1}>满减</option>
              <option value={2}>折扣</option>
              <option value={3}>赠点</option>
            </select>
          </Field>
          <Field label={form.discount_type === 2 ? '折扣百分比' : form.discount_type === 3 ? '赠送积分（点）' : '减免金额（元）'}>
            <input className="input" type="number" min={0} value={form.discount_val} onChange={(e) => set('discount_val', Number(e.target.value) || 0)} />
          </Field>
          <Field label="最低消费（元）"><input className="input" type="number" min={0} value={form.min_amount} onChange={(e) => set('min_amount', Number(e.target.value) || 0)} /></Field>
          <Field label="适用范围"><input className="input" value={form.apply_to} onChange={(e) => set('apply_to', e.target.value)} placeholder="all / p100 / image" /></Field>
          <Field label="总数量"><input className="input" type="number" min={0} value={form.total_qty} onChange={(e) => set('total_qty', Number(e.target.value) || 0)} /></Field>
          <Field label="每用户限用"><input className="input" type="number" min={0} value={form.per_user_limit} onChange={(e) => set('per_user_limit', Number(e.target.value) || 0)} /></Field>
          <Field label="开始时间"><input className="input" type="datetime-local" value={form.start_at} onChange={(e) => set('start_at', e.target.value)} /></Field>
          <Field label="结束时间"><input className="input" type="datetime-local" value={form.end_at} onChange={(e) => set('end_at', e.target.value)} /></Field>
          <Field label="状态">
            <select className="select" value={form.status} onChange={(e) => set('status', Number(e.target.value) as 0 | 1)}>
              <option value={1}>启用</option>
              <option value={0}>停用</option>
            </select>
          </Field>
        </div>
        <div className="flex justify-end gap-2">
          <button className="btn btn-outline btn-md" onClick={onClose}>取消</button>
          <button className="btn btn-primary btn-md" disabled={saving} onClick={onSave}>{saving ? '保存中...' : '保存'}</button>
        </div>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="field"><span className="field-label">{label}</span>{children}</label>;
}

function rowToForm(row: AdminPromoItem): FormState {
  return {
    id: row.id,
    code: row.code,
    name: row.name,
    discount_type: row.discount_type as 1 | 2 | 3,
    discount_val: displayDiscount(row),
    min_amount: row.min_amount / 100,
    apply_to: row.apply_to,
    total_qty: row.total_qty,
    per_user_limit: row.per_user_limit,
    start_at: toLocalInput(row.start_at),
    end_at: toLocalInput(row.end_at),
    status: row.status as 0 | 1,
  };
}

function formToBody(f: FormState): AdminPromoBody {
  return {
    code: f.code.trim(),
    name: f.name.trim(),
    discount_type: f.discount_type,
    discount_val: storedDiscount(f),
    min_amount: Math.round((Number(f.min_amount) || 0) * 100),
    apply_to: f.apply_to.trim() || 'all',
    total_qty: Number(f.total_qty) || 0,
    per_user_limit: Number(f.per_user_limit) || 0,
    start_at: fromLocalInput(f.start_at),
    end_at: fromLocalInput(f.end_at),
    status: f.status,
  };
}

function displayDiscount(row: AdminPromoItem) {
  if (row.discount_type === 1 || row.discount_type === 3) return row.discount_val / 100;
  return row.discount_val;
}

function storedDiscount(f: FormState) {
  if (f.discount_type === 1 || f.discount_type === 3) return Math.round((Number(f.discount_val) || 0) * 100);
  return Math.round(Number(f.discount_val) || 0);
}

function discountValue(row: AdminPromoItem) {
  if (row.discount_type === 1) return `${fmtNumber(row.discount_val / 100)} 元`;
  if (row.discount_type === 2) return `${row.discount_val}%`;
  return `${fmtPoints(row.discount_val)} 点`;
}

function discountLabel(v: number) {
  if (v === 1) return '满减';
  if (v === 2) return '折扣';
  if (v === 3) return '赠点';
  return String(v);
}

function toLocalInput(ts?: number) {
  if (!ts) return '';
  const d = new Date(ts * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fromLocalInput(v: string) {
  if (!v) return 0;
  return Math.floor(new Date(v).getTime() / 1000);
}
