import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Upload, RefreshCw, Trash2, Power } from 'lucide-react';
import { useMemo, useState } from 'react';

import { ApiError } from '../../lib/api';
import { fmtNumber, fmtRelative, statusLabel } from '../../lib/format';
import { accountsApi } from '../../lib/services';
import type {
  AccountBatchImportBody,
  AccountCreateBody,
  AccountItem,
} from '../../lib/types';
import { toast } from '../../stores/toast';

const TONE_CLS: Record<'ok' | 'warn' | 'err' | 'mute', string> = {
  ok: 'badge badge-success',
  warn: 'badge badge-warning',
  err: 'badge badge-danger',
  mute: 'badge',
};

export default function TokenAccountsPage() {
  const qc = useQueryClient();

  const [provider, setProvider] = useState<'all' | 'gpt' | 'grok'>('all');
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const pageSize = 20;

  const [openCreate, setOpenCreate] = useState(false);
  const [openImport, setOpenImport] = useState(false);

  const query = useMemo(
    () => ({
      provider: provider === 'all' ? undefined : provider,
      keyword: keyword || undefined,
      page,
      page_size: pageSize,
    }),
    [provider, keyword, page],
  );

  const list = useQuery({
    queryKey: ['admin', 'accounts', 'list', query],
    queryFn: () => accountsApi.list(query),
  });

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['admin', 'accounts'] });
    qc.invalidateQueries({ queryKey: ['admin', 'pool', 'stats'] });
  };

  const toggleStatus = useMutation({
    mutationFn: ({ id, status }: { id: number; status: 0 | 1 }) =>
      accountsApi.update(id, { status }),
    onSuccess: () => {
      refresh();
      toast.success('已更新');
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: number) => accountsApi.remove(id),
    onSuccess: () => {
      refresh();
      toast.success('已删除');
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const total = list.data?.total ?? 0;
  const items: AccountItem[] = list.data?.list ?? [];
  const lastPage = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">Token 管理</h1>
          <p className="page-subtitle">批量管理 GPT / GROK 账号池，支持调度策略热切换。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-outline btn-md" onClick={refresh}>
            <RefreshCw size={16} /> 刷新
          </button>
          <button className="btn btn-outline btn-md" onClick={() => setOpenImport(true)}>
            <Upload size={16} /> 批量导入
          </button>
          <button className="btn btn-primary btn-md" onClick={() => setOpenCreate(true)}>
            <Plus size={18} /> 新增账号
          </button>
        </div>
      </header>

      {/* 筛选条 */}
      <div className="card card-section flex flex-wrap items-center gap-3 !py-3">
        <div className="tabs">
          {(['all', 'gpt', 'grok'] as const).map((p) => (
            <button
              key={p}
              type="button"
              className="tab"
              aria-selected={provider === p}
              onClick={() => { setProvider(p); setPage(1); }}
            >
              {p === 'all' ? '全部' : p.toUpperCase()}
            </button>
          ))}
        </div>
        <input
          className="input flex-1 min-w-[220px]"
          placeholder="按名称 / 备注搜索…"
          value={keyword}
          onChange={(e) => { setKeyword(e.target.value); setPage(1); }}
        />
        <span className="text-small text-text-tertiary">共 {fmtNumber(total)} 条</span>
      </div>

      {/* 表格 */}
      <div className="card overflow-x-auto">
        <table className="data-table min-w-[960px]">
          <thead>
            <tr>
              <th>名称</th>
              <th>Provider</th>
              <th>凭证</th>
              <th>状态</th>
              <th>权重</th>
              <th>RPM</th>
              <th>成功 / 失败</th>
              <th>最近使用</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {list.isLoading && (
              <tr>
                <td colSpan={9} className="text-center text-text-tertiary text-small py-10">加载中…</td>
              </tr>
            )}
            {!list.isLoading && items.length === 0 && (
              <tr>
                <td colSpan={9}>
                  <div className="empty-state">
                    <p className="empty-state-title">暂无账号</p>
                    <p className="empty-state-desc">点击右上角【新增账号】或【批量导入】开始。</p>
                  </div>
                </td>
              </tr>
            )}
            {items.map((r) => {
              const s = statusLabel(r.status);
              const enabled = r.status === 1;
              return (
                <tr key={r.id}>
                  <td className="font-medium text-text-primary">
                    {r.name}
                    {r.remark && (
                      <span className="block text-small text-text-tertiary mt-0.5">{r.remark}</span>
                    )}
                  </td>
                  <td className="uppercase text-klein-500 font-semibold">{r.provider}</td>
                  <td className="font-mono text-small text-text-tertiary">{r.credential_mask}</td>
                  <td>
                    <span className={TONE_CLS[s.tone]}>{s.label}</span>
                    {r.last_error && (
                      <span
                        className="block text-small text-danger mt-1 truncate max-w-[180px]"
                        title={r.last_error}
                      >
                        {r.last_error}
                      </span>
                    )}
                  </td>
                  <td>{r.weight}</td>
                  <td>{r.rpm_limit || '∞'}</td>
                  <td className="text-small">
                    <span className="text-success">{fmtNumber(r.success_count)}</span>
                    {' / '}
                    <span className="text-danger">{fmtNumber(r.error_count)}</span>
                  </td>
                  <td className="text-small text-text-tertiary">{fmtRelative(r.last_used_at)}</td>
                  <td>
                    <div className="inline-flex gap-1">
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title={enabled ? '禁用' : '启用'}
                        onClick={() => toggleStatus.mutate({ id: r.id, status: enabled ? 0 : 1 })}
                      >
                        <Power size={14} className={enabled ? 'text-success' : 'text-text-tertiary'} />
                      </button>
                      <button
                        className="btn btn-danger-ghost btn-icon btn-sm"
                        title="删除"
                        onClick={() => {
                          if (confirm(`确定删除账号「${r.name}」？`)) {
                            remove.mutate(r.id);
                          }
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

      {/* 分页 */}
      {total > pageSize && (
        <div className="flex justify-end items-center gap-2 text-small">
          <button
            className="btn btn-outline btn-sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            上一页
          </button>
          <span className="text-text-tertiary">
            {page} / {lastPage}
          </span>
          <button
            className="btn btn-outline btn-sm"
            disabled={page >= lastPage}
            onClick={() => setPage((p) => Math.min(lastPage, p + 1))}
          >
            下一页
          </button>
        </div>
      )}

      {openCreate && (
        <CreateDialog
          onClose={() => setOpenCreate(false)}
          onSuccess={() => {
            setOpenCreate(false);
            refresh();
          }}
        />
      )}
      {openImport && (
        <ImportDialog
          onClose={() => setOpenImport(false)}
          onSuccess={() => {
            setOpenImport(false);
            refresh();
          }}
        />
      )}
    </div>
  );
}

// ============== Create Dialog ==============
function CreateDialog({ onClose, onSuccess }: { onClose: () => void; onSuccess: () => void }) {
  const [body, setBody] = useState<AccountCreateBody>({
    provider: 'gpt',
    name: '',
    auth_type: 'api_key',
    credential: '',
    base_url: '',
    weight: 10,
    remark: '',
  });

  const m = useMutation({
    mutationFn: (b: AccountCreateBody) => accountsApi.create(b),
    onSuccess: () => {
      toast.success('账号已添加');
      onSuccess();
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  return (
    <Modal title="新增账号" onClose={onClose}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (!body.name.trim() || !body.credential.trim()) {
            toast.error('请填写名称和凭证');
            return;
          }
          m.mutate({
            ...body,
            base_url: body.base_url?.trim() || undefined,
            remark: body.remark?.trim() || undefined,
          });
        }}
      >
        <Field label="Provider">
          <select
            className="select"
            value={body.provider}
            onChange={(e) => setBody((s) => ({ ...s, provider: e.target.value as 'gpt' | 'grok' }))}
          >
            <option value="gpt">GPT (生图)</option>
            <option value="grok">GROK (生视频)</option>
          </select>
        </Field>

        <Field label="名称 / 标签">
          <input
            className="input"
            placeholder="如 GPT-Acc-001"
            value={body.name}
            onChange={(e) => setBody((s) => ({ ...s, name: e.target.value }))}
          />
        </Field>

        <Field label="凭证类型">
          <select
            className="select"
            value={body.auth_type}
            onChange={(e) =>
              setBody((s) => ({ ...s, auth_type: e.target.value as 'api_key' | 'cookie' | 'oauth' }))
            }
          >
            <option value="api_key">API Key</option>
            <option value="cookie">Cookie</option>
            <option value="oauth">OAuth Token</option>
          </select>
        </Field>

        <Field label="凭证（明文，写库前自动 AES-256-GCM 加密）">
          <textarea
            className="textarea font-mono text-small"
            placeholder="sk-xxxx... 或 cookie 全文"
            value={body.credential}
            onChange={(e) => setBody((s) => ({ ...s, credential: e.target.value }))}
          />
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Base URL（可选）">
            <input
              className="input"
              placeholder="https://api.openai.com"
              value={body.base_url || ''}
              onChange={(e) => setBody((s) => ({ ...s, base_url: e.target.value }))}
            />
          </Field>
          <Field label="权重">
            <input
              type="number"
              className="input"
              min={1}
              max={1000}
              value={body.weight ?? 10}
              onChange={(e) => setBody((s) => ({ ...s, weight: Number(e.target.value) || 10 }))}
            />
          </Field>
        </div>

        <Field label="备注（可选）">
          <input
            className="input"
            value={body.remark || ''}
            onChange={(e) => setBody((s) => ({ ...s, remark: e.target.value }))}
          />
        </Field>

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn btn-outline btn-md" onClick={onClose}>取消</button>
          <button type="submit" className="btn btn-primary btn-md" disabled={m.isPending}>
            {m.isPending ? '提交中…' : '保存'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

// ============== Import Dialog ==============
function ImportDialog({ onClose, onSuccess }: { onClose: () => void; onSuccess: () => void }) {
  const [body, setBody] = useState<AccountBatchImportBody>({
    provider: 'gpt',
    auth_type: 'api_key',
    base_url: '',
    weight: 10,
    text: '',
  });

  const m = useMutation({
    mutationFn: (b: AccountBatchImportBody) => accountsApi.batchImport(b),
    onSuccess: (r) => {
      toast.success(`成功导入 ${r.imported} 条`);
      onSuccess();
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  return (
    <Modal title="批量导入账号" onClose={onClose}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (!body.text.trim()) {
            toast.error('请粘贴账号列表');
            return;
          }
          m.mutate({
            ...body,
            base_url: body.base_url?.trim() || undefined,
          });
        }}
      >
        <div className="grid grid-cols-3 gap-3">
          <Field label="Provider">
            <select
              className="select"
              value={body.provider}
              onChange={(e) => setBody((s) => ({ ...s, provider: e.target.value as 'gpt' | 'grok' }))}
            >
              <option value="gpt">GPT</option>
              <option value="grok">GROK</option>
            </select>
          </Field>
          <Field label="凭证类型">
            <select
              className="select"
              value={body.auth_type}
              onChange={(e) =>
                setBody((s) => ({ ...s, auth_type: e.target.value as 'api_key' | 'cookie' | 'oauth' }))
              }
            >
              <option value="api_key">API Key</option>
              <option value="cookie">Cookie</option>
              <option value="oauth">OAuth</option>
            </select>
          </Field>
          <Field label="默认权重">
            <input
              type="number"
              className="input"
              min={1}
              max={1000}
              value={body.weight ?? 10}
              onChange={(e) => setBody((s) => ({ ...s, weight: Number(e.target.value) || 10 }))}
            />
          </Field>
        </div>

        <Field label="默认 Base URL（可选）">
          <input
            className="input"
            placeholder="https://api.openai.com"
            value={body.base_url || ''}
            onChange={(e) => setBody((s) => ({ ...s, base_url: e.target.value }))}
          />
        </Field>

        <Field
          label="账号列表（每行一条）"
          hint={
            <span>
              支持三种格式：
              <code className="kbd mx-1">{'<name>@@<credential>'}</code>
              <code className="kbd mx-1">{'<credential>@<base_url>'}</code>
              <code className="kbd mx-1">{'<credential>'}</code>
              。空行 / # 开头将忽略。
            </span>
          }
        >
          <textarea
            className="textarea font-mono text-small min-h-[160px]"
            placeholder={'GPT-001@@sk-aaaaaa\nsk-bbbbbb@https://api.example.com\nsk-cccccc'}
            value={body.text}
            onChange={(e) => setBody((s) => ({ ...s, text: e.target.value }))}
          />
        </Field>

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn btn-outline btn-md" onClick={onClose}>取消</button>
          <button type="submit" className="btn btn-primary btn-md" disabled={m.isPending}>
            {m.isPending ? '导入中…' : '开始导入'}
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
          <button className="btn btn-ghost btn-icon btn-sm" onClick={onClose} aria-label="关闭">
            ✕
          </button>
        </header>
        <div className="p-5 max-h-[70vh] overflow-y-auto">{children}</div>
      </div>
    </div>
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <label className="field">
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}
