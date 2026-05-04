import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  CheckCircle2,
  Clock,
  Pencil,
  Plus,
  Power,
  RefreshCw,
  Trash2,
  Upload,
  XCircle,
} from 'lucide-react';
import { type FormEvent, type ReactNode, useEffect, useMemo, useRef, useState } from 'react';

import { ApiError } from '../../lib/api';
import { fmtRelative, fmtTime } from '../../lib/format';
import { proxiesApi } from '../../lib/services';
import type { ProxyCreateBody, ProxyItem, ProxyUpdateBody } from '../../lib/types';
import { toast } from '../../stores/toast';

const PROTOCOLS: ProxyCreateBody['protocol'][] = ['http', 'https', 'socks5', 'socks5h'];

function checkLabel(status?: number): { label: string; cls: string; icon: typeof CheckCircle2 } {
  switch (status) {
    case 1:
      return { label: '可用', cls: 'text-success', icon: CheckCircle2 };
    case 2:
      return { label: '失败', cls: 'text-danger', icon: XCircle };
    default:
      return { label: '未测', cls: 'text-text-tertiary', icon: Clock };
  }
}

export default function ProxiesPage() {
  const qc = useQueryClient();

  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [editor, setEditor] = useState<{ mode: 'create' } | { mode: 'edit'; row: ProxyItem } | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const headerCbRef = useRef<HTMLInputElement | null>(null);
  const pageSize = 20;

  const query = useMemo(
    () => ({
      keyword: keyword.trim() || undefined,
      status: statusFilter === 'all' ? undefined : statusFilter === 'enabled' ? (1 as const) : (0 as const),
      page,
      page_size: pageSize,
    }),
    [keyword, page, statusFilter],
  );

  const list = useQuery({
    queryKey: ['admin', 'proxies', 'list', query],
    queryFn: () => proxiesApi.list(query),
  });

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['admin', 'proxies'] });
  };

  const toggle = useMutation({
    mutationFn: ({ id, status }: { id: number; status: 0 | 1 }) => proxiesApi.update(id, { status }),
    onSuccess: () => {
      refresh();
      toast.success('代理状态已更新');
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: number) => proxiesApi.remove(id),
    onSuccess: () => {
      refresh();
      setSelected((prev) => {
        const next = new Set(prev);
        if (remove.variables) next.delete(remove.variables);
        return next;
      });
      toast.success('代理已删除');
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const batchDelete = useMutation({
    mutationFn: (ids: number[]) => proxiesApi.batchDelete(ids),
    onSuccess: (res) => {
      refresh();
      setSelected(new Set());
      toast.success(`已删除 ${res.deleted} 个代理`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const testMut = useMutation({
    mutationFn: (id: number) => proxiesApi.test(id),
    onSuccess: (r) => {
      refresh();
      if (r.ok) {
        toast.success(`代理可用，延迟 ${r.latency_ms}ms`);
      } else {
        toast.error(`代理不可用：${r.error || '未知错误'}`);
      }
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const batchTest = useMutation({
    mutationFn: (ids: number[]) => proxiesApi.batchTest(ids),
    onSuccess: (r) => {
      refresh();
      toast.success(`已测试 ${r.tested} 个代理，可用 ${r.ok} 个，失败 ${r.failed} 个`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const total = list.data?.total ?? 0;
  const items = list.data?.list ?? [];
  const pageIds = items.map((item) => item.id);
  const pageAllSelected = pageIds.length > 0 && pageIds.every((id) => selected.has(id));
  const selectedCount = selected.size;
  const lastPage = Math.max(1, Math.ceil(total / pageSize));

  useEffect(() => {
    const el = headerCbRef.current;
    if (!el) return;
    const some = pageIds.some((id) => selected.has(id));
    el.indeterminate = some && !pageAllSelected;
  }, [pageAllSelected, pageIds, selected]);

  useEffect(() => {
    setSelected(new Set());
  }, [keyword, statusFilter]);

  const toggleSelect = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectPage = () => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (pageAllSelected) {
        pageIds.forEach((id) => next.delete(id));
      } else {
        pageIds.forEach((id) => next.add(id));
      }
      return next;
    });
  };

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">代理管理</h1>
          <p className="page-subtitle">统一维护账号可用代理，支持批量导入、批量测试和批量删除。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-outline btn-md" onClick={refresh}>
            <RefreshCw size={16} /> 刷新
          </button>
          <button className="btn btn-outline btn-md" onClick={() => setImportOpen(true)}>
            <Upload size={16} /> 批量添加
          </button>
          <button
            className="btn btn-outline btn-md"
            disabled={selectedCount === 0 || batchTest.isPending}
            onClick={() => batchTest.mutate([...selected])}
          >
            <Activity size={16} className={batchTest.isPending ? 'animate-pulse' : ''} />
            批量测试
          </button>
          <button
            className="btn btn-danger btn-md"
            disabled={selectedCount === 0 || batchDelete.isPending}
            onClick={() => {
              if (!confirm(`确认删除选中的 ${selectedCount} 个代理吗？`)) return;
              batchDelete.mutate([...selected]);
            }}
          >
            <Trash2 size={16} /> 批量删除
          </button>
          <button className="btn btn-primary btn-md" onClick={() => setEditor({ mode: 'create' })}>
            <Plus size={18} /> 新增代理
          </button>
        </div>
      </header>

      <div className="card card-section flex flex-wrap items-center gap-3 !py-3">
        <div className="tabs">
          {(['all', 'enabled', 'disabled'] as const).map((item) => (
            <button
              key={item}
              type="button"
              className="tab"
              aria-selected={statusFilter === item}
              onClick={() => {
                setStatusFilter(item);
                setPage(1);
              }}
            >
              {item === 'all' ? '全部' : item === 'enabled' ? '启用' : '禁用'}
            </button>
          ))}
        </div>
        <input
          className="input flex-1 min-w-[220px]"
          placeholder="搜索名称、主机、备注"
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value);
            setPage(1);
          }}
        />
        <span className="text-small text-text-tertiary">
          共 {total} 条，已选 {selectedCount} 条
        </span>
      </div>

      <div className="card overflow-x-auto">
        <table className="data-table min-w-[1180px]">
          <thead>
            <tr>
              <th className="w-10">
                <input
                  ref={headerCbRef}
                  type="checkbox"
                  className="rounded border-border"
                  checked={pageAllSelected}
                  onChange={toggleSelectPage}
                  disabled={list.isLoading || items.length === 0}
                  title="全选当前页"
                />
              </th>
              <th>名称</th>
              <th>协议</th>
              <th>地址</th>
              <th>认证</th>
              <th>状态</th>
              <th>最近测试</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {list.isLoading && (
              <tr>
                <td colSpan={8} className="py-10 text-center text-small text-text-tertiary">
                  加载中…
                </td>
              </tr>
            )}
            {!list.isLoading && items.length === 0 && (
              <tr>
                <td colSpan={8}>
                  <div className="empty-state">
                    <p className="empty-state-title">暂无代理</p>
                    <p className="empty-state-desc">可以手动新增，也可以在批量添加里一次导入多条代理。</p>
                  </div>
                </td>
              </tr>
            )}
            {items.map((item) => {
              const enabled = item.status === 1;
              const test = checkLabel(item.last_check_ok);
              const TestIcon = test.icon;
              return (
                <tr key={item.id}>
                  <td className="w-10">
                    <input
                      type="checkbox"
                      className="rounded border-border"
                      checked={selected.has(item.id)}
                      onChange={() => toggleSelect(item.id)}
                      aria-label={`选择代理 ${item.name}`}
                    />
                  </td>
                  <td className="font-medium text-text-primary">
                    {item.name}
                    {item.remark && (
                      <span className="mt-0.5 block text-small text-text-tertiary">{item.remark}</span>
                    )}
                  </td>
                  <td className="font-semibold uppercase text-klein-500">{item.protocol}</td>
                  <td className="font-mono text-small text-text-secondary">
                    {item.host}:{item.port}
                  </td>
                  <td className="text-small">
                    {item.username ? (
                      <span>
                        {item.username}
                        {item.has_password && <span className="text-text-tertiary"> / 已设密码</span>}
                      </span>
                    ) : (
                      <span className="text-text-tertiary">无认证</span>
                    )}
                  </td>
                  <td>{enabled ? <span className="badge badge-success">启用</span> : <span className="badge">禁用</span>}</td>
                  <td className="text-small">
                    <div className={`inline-flex items-center gap-1 ${test.cls}`}>
                      <TestIcon size={12} />
                      <span>
                        {test.label}
                        {item.last_check_ms ? ` / ${item.last_check_ms}ms` : ''}
                      </span>
                    </div>
                    {item.last_check_at && (
                      <span className="mt-0.5 block text-tiny text-text-tertiary" title={fmtTime(item.last_check_at)}>
                        {fmtRelative(item.last_check_at)}
                      </span>
                    )}
                    {item.last_error && (
                      <span className="mt-0.5 block max-w-[220px] truncate text-tiny text-danger" title={item.last_error}>
                        {item.last_error}
                      </span>
                    )}
                  </td>
                  <td>
                    <div className="inline-flex gap-1">
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title="测试代理"
                        disabled={testMut.isPending && testMut.variables === item.id}
                        onClick={() => testMut.mutate(item.id)}
                      >
                        <Activity
                          size={14}
                          className={testMut.isPending && testMut.variables === item.id ? 'animate-pulse text-klein-500' : 'text-text-secondary'}
                        />
                      </button>
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title="编辑"
                        onClick={() => setEditor({ mode: 'edit', row: item })}
                      >
                        <Pencil size={14} />
                      </button>
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title={enabled ? '禁用' : '启用'}
                        onClick={() => toggle.mutate({ id: item.id, status: enabled ? 0 : 1 })}
                      >
                        <Power size={14} className={enabled ? 'text-success' : 'text-text-tertiary'} />
                      </button>
                      <button
                        className="btn btn-danger-ghost btn-icon btn-sm"
                        title="删除"
                        onClick={() => {
                          if (!confirm(`确认删除代理“${item.name}”吗？`)) return;
                          remove.mutate(item.id);
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
        <div className="flex items-center justify-end gap-2 text-small">
          <button className="btn btn-outline btn-sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
            上一页
          </button>
          <span className="text-text-tertiary">
            {page} / {lastPage}
          </span>
          <button className="btn btn-outline btn-sm" disabled={page >= lastPage} onClick={() => setPage((p) => Math.min(lastPage, p + 1))}>
            下一页
          </button>
        </div>
      )}

      {editor && (
        <ProxyDialog
          mode={editor.mode}
          row={editor.mode === 'edit' ? editor.row : undefined}
          onClose={() => setEditor(null)}
          onSuccess={() => {
            setEditor(null);
            refresh();
          }}
        />
      )}

      {importOpen && (
        <BatchImportDialog
          onClose={() => setImportOpen(false)}
          onSuccess={() => {
            setImportOpen(false);
            refresh();
          }}
        />
      )}
    </div>
  );
}

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
    mutationFn: (payload: ProxyCreateBody) => proxiesApi.create(payload),
    onSuccess: () => {
      toast.success('代理已添加');
      onSuccess();
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const update = useMutation({
    mutationFn: (payload: ProxyUpdateBody) => proxiesApi.update(row!.id, payload),
    onSuccess: () => {
      toast.success('代理已更新');
      onSuccess();
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!body.name.trim() || !body.host.trim() || !body.port) {
      toast.error('请填写名称、主机和端口');
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
      return;
    }
    const patch: ProxyUpdateBody = {
      name: payload.name,
      protocol: payload.protocol,
      host: payload.host,
      port: payload.port,
      username: payload.username,
      remark: payload.remark,
    };
    if (body.password) patch.password = body.password;
    update.mutate(patch);
  };

  const submitting = create.isPending || update.isPending;

  return (
    <Modal title={mode === 'create' ? '新增代理' : '编辑代理'} onClose={onClose}>
      <form className="space-y-3" onSubmit={submit}>
        <div className="grid grid-cols-2 gap-3">
          <Field label="名称">
            <input className="input" value={body.name} onChange={(e) => setBody((s) => ({ ...s, name: e.target.value }))} placeholder="例如：香港-01" />
          </Field>
          <Field label="协议">
            <select
              className="select"
              value={body.protocol}
              onChange={(e) => setBody((s) => ({ ...s, protocol: e.target.value as ProxyCreateBody['protocol'] }))}
            >
              {PROTOCOLS.map((proto) => (
                <option key={proto} value={proto}>
                  {proto}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <div className="grid grid-cols-3 gap-3">
          <Field label="主机" className="col-span-2">
            <input className="input" value={body.host} onChange={(e) => setBody((s) => ({ ...s, host: e.target.value }))} placeholder="proxy.example.com" />
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
          <Field label="用户名">
            <input className="input" value={body.username || ''} onChange={(e) => setBody((s) => ({ ...s, username: e.target.value }))} placeholder="可选" />
          </Field>
          <Field label={mode === 'edit' ? '密码（留空表示不变）' : '密码'}>
            <input type="password" className="input" value={body.password || ''} onChange={(e) => setBody((s) => ({ ...s, password: e.target.value }))} placeholder="可选" />
          </Field>
        </div>

        <Field label="备注">
          <input className="input" value={body.remark || ''} onChange={(e) => setBody((s) => ({ ...s, remark: e.target.value }))} />
        </Field>

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn btn-outline btn-md" onClick={onClose}>
            取消
          </button>
          <button type="submit" className="btn btn-primary btn-md" disabled={submitting}>
            {submitting ? '提交中…' : '保存'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function BatchImportDialog({ onClose, onSuccess }: { onClose: () => void; onSuccess: () => void }) {
  const [text, setText] = useState('');

  const importMut = useMutation({
    mutationFn: (payload: { text: string }) => proxiesApi.batchImport(payload),
    onSuccess: (res) => {
      const details = [
        `导入 ${res.created} 条`,
        res.skipped ? `跳过 ${res.skipped} 条` : '',
        res.failed ? `失败 ${res.failed} 条` : '',
      ]
        .filter(Boolean)
        .join('，');
      toast.success(details);
      if (res.errors?.length) {
        toast.info(`部分失败：${res.errors.slice(0, 3).join('；')}`);
      }
      onSuccess();
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  return (
    <Modal title="批量添加代理" onClose={onClose}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (!text.trim()) {
            toast.error('请先粘贴代理列表');
            return;
          }
          importMut.mutate({ text });
        }}
      >
        <Field
          label="每行一个代理 URI"
          hint={
            <>
              支持 `protocol://host:port`、`protocol://user:pass@host:port`，可选追加 `#名称`。
            </>
          }
        >
          <textarea
            className="textarea min-h-[220px] font-mono text-small"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={[
              'http://127.0.0.1:7890',
              'socks5://user:pass@127.0.0.1:1080#日本节点',
              'https://proxy.example.com:443#备用出口',
            ].join('\n')}
          />
        </Field>

        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn btn-outline btn-md" onClick={onClose}>
            取消
          </button>
          <button type="submit" className="btn btn-primary btn-md" disabled={importMut.isPending}>
            {importMut.isPending ? '导入中…' : '开始导入'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-[80] grid place-items-center bg-black/40 p-4 backdrop-blur-sm">
      <div className="dialog-surface w-full max-w-xl klein-fade-in">
        <header className="flex h-12 items-center justify-between border-b border-border px-5">
          <h3 className="font-semibold text-text-primary">{title}</h3>
          <button className="btn btn-ghost btn-icon btn-sm" onClick={onClose} aria-label="关闭">
            ×
          </button>
        </header>
        <div className="max-h-[70vh] overflow-y-auto p-5">{children}</div>
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
  hint?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <label className={`field ${className || ''}`}>
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}
