import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Plus, RefreshCw, Trash2, Power, Activity, Pencil, CheckCircle2, XCircle, Clock,
} from 'lucide-react';
import { useMemo, useState } from 'react';

import { ApiError } from '../../lib/api';
import { fmtRelative, fmtTime } from '../../lib/format';
import { proxiesApi } from '../../lib/services';
import type { ProxyCreateBody, ProxyItem, ProxyUpdateBody } from '../../lib/types';
import { toast } from '../../stores/toast';

const PROTOS: ProxyCreateBody['protocol'][] = ['http', 'https', 'socks5', 'socks5h'];

function checkLabel(s?: number): { label: string; cls: string; icon: typeof CheckCircle2 } {
  switch (s) {
    case 1: return { label: 'OK',   cls: 'text-success',       icon: CheckCircle2 };
    case 2: return { label: 'FAIL', cls: 'text-danger',        icon: XCircle };
    default: return { label: '未测', cls: 'text-text-tertiary', icon: Clock };
  }
}

export default function ProxiesPage() {
  const qc = useQueryClient();

  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const [page, setPage] = useState(1);
  const pageSize = 20;

  const [openDlg, setOpenDlg] = useState<{ mode: 'create' } | { mode: 'edit'; row: ProxyItem } | null>(null);

  const query = useMemo(
    () => ({
      keyword: keyword || undefined,
      status: statusFilter === 'all' ? undefined : statusFilter === 'enabled' ? (1 as const) : (0 as const),
      page,
      page_size: pageSize,
    }),
    [keyword, statusFilter, page],
  );

  const list = useQuery({
    queryKey: ['admin', 'proxies', 'list', query],
    queryFn: () => proxiesApi.list(query),
  });

  const refresh = () => qc.invalidateQueries({ queryKey: ['admin', 'proxies'] });

  const toggle = useMutation({
    mutationFn: ({ id, status }: { id: number; status: 0 | 1 }) =>
      proxiesApi.update(id, { status }),
    onSuccess: () => { refresh(); toast.success('已更新'); },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: number) => proxiesApi.remove(id),
    onSuccess: () => { refresh(); toast.success('已删除'); },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const testMut = useMutation({
    mutationFn: (id: number) => proxiesApi.test(id),
    onSuccess: (r) => {
      refresh();
      if (r.ok) toast.success(`代理可达 · ${r.latency_ms}ms`);
      else toast.error(`不可用：${r.error || '未知'}`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const total = list.data?.total ?? 0;
  const items: ProxyItem[] = list.data?.list ?? [];
  const lastPage = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">代理管理</h1>
          <p className="page-subtitle">为账号池配置 HTTP / SOCKS5 代理；可在系统配置中设为全局默认。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-outline btn-md" onClick={refresh}>
            <RefreshCw size={16} /> 刷新
          </button>
          <button className="btn btn-primary btn-md" onClick={() => setOpenDlg({ mode: 'create' })}>
            <Plus size={18} /> 新增代理
          </button>
        </div>
      </header>

      <div className="card card-section flex flex-wrap items-center gap-3 !py-3">
        <div className="tabs">
          {(['all', 'enabled', 'disabled'] as const).map((p) => (
            <button
              key={p}
              type="button"
              className="tab"
              aria-selected={statusFilter === p}
              onClick={() => { setStatusFilter(p); setPage(1); }}
            >
              {p === 'all' ? '全部' : p === 'enabled' ? '启用' : '禁用'}
            </button>
          ))}
        </div>
        <input
          className="input flex-1 min-w-[220px]"
          placeholder="按名称 / 备注 / 主机搜索…"
          value={keyword}
          onChange={(e) => { setKeyword(e.target.value); setPage(1); }}
        />
        <span className="text-small text-text-tertiary">共 {total} 条</span>
      </div>

      <div className="card overflow-x-auto">
        <table className="data-table min-w-[1080px]">
          <thead>
            <tr>
              <th>名称</th>
              <th>协议</th>
              <th>地址</th>
              <th>认证</th>
              <th>状态</th>
              <th>最近探测</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {list.isLoading && (
              <tr>
                <td colSpan={7} className="text-center text-text-tertiary text-small py-10">加载中…</td>
              </tr>
            )}
            {!list.isLoading && items.length === 0 && (
              <tr>
                <td colSpan={7}>
                  <div className="empty-state">
                    <p className="empty-state-title">暂无代理</p>
                    <p className="empty-state-desc">点击右上角【新增代理】添加一条 HTTP / SOCKS5 出口。</p>
                  </div>
                </td>
              </tr>
            )}
            {items.map((p) => {
              const enabled = p.status === 1;
              const t = checkLabel(p.last_check_ok);
              const Icon = t.icon;
              return (
                <tr key={p.id}>
                  <td className="font-medium text-text-primary">
                    {p.name}
                    {p.remark && (
                      <span className="block text-small text-text-tertiary mt-0.5">{p.remark}</span>
                    )}
                  </td>
                  <td className="uppercase text-klein-500 font-semibold">{p.protocol}</td>
                  <td className="font-mono text-small text-text-secondary">{p.host}:{p.port}</td>
                  <td className="text-small">
                    {p.username ? (
                      <span>
                        {p.username}
                        {p.has_password && <span className="text-text-tertiary"> · ●●●</span>}
                      </span>
                    ) : (
                      <span className="text-text-tertiary">无</span>
                    )}
                  </td>
                  <td>
                    {enabled ? (
                      <span className="badge badge-success">启用</span>
                    ) : (
                      <span className="badge">禁用</span>
                    )}
                  </td>
                  <td className="text-small">
                    <div className={`inline-flex items-center gap-1 ${t.cls}`}>
                      <Icon size={12} />
                      <span>
                        {t.label}
                        {p.last_check_ms ? ` · ${p.last_check_ms}ms` : ''}
                      </span>
                    </div>
                    {p.last_check_at && (
                      <span className="block text-tiny text-text-tertiary mt-0.5" title={fmtTime(p.last_check_at)}>
                        {fmtRelative(p.last_check_at)}
                      </span>
                    )}
                    {p.last_error && (
                      <span className="block text-tiny text-danger mt-0.5 truncate max-w-[220px]" title={p.last_error}>
                        {p.last_error}
                      </span>
                    )}
                  </td>
                  <td>
                    <div className="inline-flex gap-1">
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title="测试连通"
                        onClick={() => testMut.mutate(p.id)}
                        disabled={testMut.isPending && testMut.variables === p.id}
                      >
                        <Activity
                          size={14}
                          className={
                            testMut.isPending && testMut.variables === p.id
                              ? 'animate-pulse text-klein-500'
                              : 'text-text-secondary'
                          }
                        />
                      </button>
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title="编辑"
                        onClick={() => setOpenDlg({ mode: 'edit', row: p })}
                      >
                        <Pencil size={14} />
                      </button>
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title={enabled ? '禁用' : '启用'}
                        onClick={() => toggle.mutate({ id: p.id, status: enabled ? 0 : 1 })}
                      >
                        <Power size={14} className={enabled ? 'text-success' : 'text-text-tertiary'} />
                      </button>
                      <button
                        className="btn btn-danger-ghost btn-icon btn-sm"
                        title="删除"
                        onClick={() => {
                          if (confirm(`确定删除代理「${p.name}」？`)) remove.mutate(p.id);
                        }}
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {total > pageSize && (
        <div className="flex justify-end items-center gap-2 text-small">
          <button
            className="btn btn-outline btn-sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >上一页</button>
          <span className="text-text-tertiary">{page} / {lastPage}</span>
          <button
            className="btn btn-outline btn-sm"
            disabled={page >= lastPage}
            onClick={() => setPage((p) => Math.min(lastPage, p + 1))}
          >下一页</button>
        </div>
      )}

      {openDlg && (
        <ProxyDialog
          mode={openDlg.mode}
          row={openDlg.mode === 'edit' ? openDlg.row : undefined}
          onClose={() => setOpenDlg(null)}
          onSuccess={() => { setOpenDlg(null); refresh(); }}
        />
      )}
    </div>
  );
}

// ============== Create / Edit Dialog ==============
function ProxyDialog({
  mode,
  row,
  onClose,
  onSuccess,
}: {
  mode: 'create' | 'edit';
  row?: ProxyItem;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [body, setBody] = useState<ProxyCreateBody>(() =>
    row
      ? {
          name: row.name,
          protocol: (row.protocol as ProxyCreateBody['protocol']) || 'http',
          host: row.host,
          port: row.port,
          username: row.username || '',
          password: '',
          remark: row.remark || '',
        }
      : { name: '', protocol: 'http', host: '', port: 7890, username: '', password: '', remark: '' },
  );

  const create = useMutation({
    mutationFn: (b: ProxyCreateBody) => proxiesApi.create(b),
    onSuccess: () => { toast.success('代理已添加'); onSuccess(); },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const update = useMutation({
    mutationFn: (b: ProxyUpdateBody) => proxiesApi.update(row!.id, b),
    onSuccess: () => { toast.success('已更新'); onSuccess(); },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!body.name.trim() || !body.host.trim() || !body.port) {
      toast.error('请填写名称 / 主机 / 端口');
      return;
    }
    const payload: ProxyCreateBody = {
      ...body,
      name: body.name.trim(),
      host: body.host.trim(),
      username: body.username?.trim() || undefined,
      password: body.password || undefined,
      remark: body.remark?.trim() || undefined,
    };
    if (mode === 'create') {
      create.mutate(payload);
    } else {
      const patch: ProxyUpdateBody = {
        name: payload.name,
        protocol: payload.protocol,
        host: payload.host,
        port: payload.port,
        username: payload.username,
        remark: payload.remark,
      };
      // password 留空表示不变
      if (body.password) patch.password = body.password;
      update.mutate(patch);
    }
  };

  const submitting = create.isPending || update.isPending;

  return (
    <Modal title={mode === 'create' ? '新增代理' : '编辑代理'} onClose={onClose}>
      <form className="space-y-3" onSubmit={submit}>
        <div className="grid grid-cols-2 gap-3">
          <Field label="名称">
            <input
              className="input"
              placeholder="如 HK-Cloudflare-1"
              value={body.name}
              onChange={(e) => setBody((s) => ({ ...s, name: e.target.value }))}
            />
          </Field>
          <Field label="协议">
            <select
              className="select"
              value={body.protocol}
              onChange={(e) =>
                setBody((s) => ({ ...s, protocol: e.target.value as ProxyCreateBody['protocol'] }))
              }
            >
              {PROTOS.map((p) => (
                <option key={p} value={p}>{p}</option>
              ))}
            </select>
          </Field>
        </div>

        <div className="grid grid-cols-3 gap-3">
          <Field label="主机 (host)" className="col-span-2">
            <input
              className="input"
              placeholder="proxy.example.com"
              value={body.host}
              onChange={(e) => setBody((s) => ({ ...s, host: e.target.value }))}
            />
          </Field>
          <Field label="端口">
            <input
              type="number"
              className="input"
              min={1}
              max={65535}
              value={body.port || ''}
              onChange={(e) => setBody((s) => ({ ...s, port: Number(e.target.value) || 0 }))}
            />
          </Field>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <Field label="用户名（可选）">
            <input
              className="input"
              value={body.username || ''}
              onChange={(e) => setBody((s) => ({ ...s, username: e.target.value }))}
            />
          </Field>
          <Field label={mode === 'edit' ? '密码（留空保持不变）' : '密码（可选）'}>
            <input
              type="password"
              className="input"
              value={body.password || ''}
              onChange={(e) => setBody((s) => ({ ...s, password: e.target.value }))}
            />
          </Field>
        </div>

        <Field label="备注">
          <input
            className="input"
            value={body.remark || ''}
            onChange={(e) => setBody((s) => ({ ...s, remark: e.target.value }))}
          />
        </Field>

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn btn-outline btn-md" onClick={onClose}>取消</button>
          <button type="submit" className="btn btn-primary btn-md" disabled={submitting}>
            {submitting ? '提交中…' : '保存'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

// ============== UI helpers ==============
function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-[80] grid place-items-center bg-black/40 backdrop-blur-sm p-4">
      <div className="dialog-surface w-full max-w-xl klein-fade-in">
        <header className="flex items-center justify-between px-5 h-12 border-b border-border">
          <h3 className="font-semibold text-text-primary">{title}</h3>
          <button className="btn btn-ghost btn-icon btn-sm" onClick={onClose} aria-label="关闭">✕</button>
        </header>
        <div className="p-5 max-h-[70vh] overflow-y-auto">{children}</div>
      </div>
    </div>
  );
}

function Field({
  label,
  hint,
  className,
  children,
}: {
  label: string;
  hint?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label className={`field ${className || ''}`}>
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}
