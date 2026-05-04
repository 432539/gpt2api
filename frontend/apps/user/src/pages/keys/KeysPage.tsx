import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import clsx from 'clsx';
import { Plus, Copy, Check, Trash2, Power, X, KeyRound } from 'lucide-react';

import { ApiError } from '../../lib/api';
import { fmtRelative } from '../../lib/format';
import { keysApi } from '../../lib/services';
import type { APIKeyCreated } from '../../lib/types';
import { toast } from '../../stores/toast';

const STATUS_ENABLED = 1;

const SCOPE_OPTIONS = [
  { value: 'image,video,chat', label: '全部能力' },
  { value: 'image', label: '仅图像' },
  { value: 'video', label: '仅视频' },
  { value: 'image,video', label: '图像 + 视频' },
];

export default function KeysPage() {
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [createdKey, setCreatedKey] = useState<APIKeyCreated | null>(null);

  const listQ = useQuery({
    queryKey: ['keys'],
    queryFn: () => keysApi.list(),
  });

  const createMut = useMutation({
    mutationFn: keysApi.create,
    onSuccess: (data) => {
      setCreatedKey(data);
      setShowCreate(false);
      qc.invalidateQueries({ queryKey: ['keys'] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '创建失败'),
  });

  const toggleMut = useMutation({
    mutationFn: keysApi.toggle,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['keys'] });
      toast.success('状态已更新');
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '操作失败'),
  });

  const removeMut = useMutation({
    mutationFn: keysApi.remove,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['keys'] });
      toast.success('已删除');
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '删除失败'),
  });

  const keys = listQ.data ?? [];

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1 className="page-title">API KEY 管理</h1>
          <p className="page-subtitle">通过 OpenAI 兼容协议直接调用当前服务（仅创建时返回明文，请妥善保存）。</p>
        </div>
        <button className="btn btn-primary btn-lg" onClick={() => setShowCreate(true)}>
          <Plus size={18} /> 创建 KEY
        </button>
      </header>

      {createdKey && <CreatedKeyCard data={createdKey} onClose={() => setCreatedKey(null)} />}

      <div className="card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="data-table min-w-[760px]">
            <thead>
              <tr>
                <th>名称</th>
                <th>KEY</th>
                <th>权限</th>
                <th>RPM</th>
                <th>最近使用</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {listQ.isLoading && (
                <tr>
                  <td colSpan={6} className="text-center text-text-tertiary text-small py-10">加载中…</td>
                </tr>
              )}
              {!listQ.isLoading && keys.length === 0 && (
                <tr>
                  <td colSpan={6}>
                    <div className="empty-state">
                      <span className="empty-state-icon">
                        <KeyRound size={22} />
                      </span>
                      <p className="empty-state-title">还没有 KEY</p>
                      <p className="empty-state-desc">点击右上角「创建 KEY」即可生成首个 API Key，开启 OpenAI 兼容调用。</p>
                    </div>
                  </td>
                </tr>
              )}
              {keys.map((k) => (
                <tr key={k.id}>
                  <td className="font-medium text-text-primary">{k.name}</td>
                  <td className="font-mono text-small text-text-secondary">{k.mask}</td>
                  <td className="text-text-secondary">{k.scope || '全部'}</td>
                  <td className="text-text-secondary">{k.rpm_limit || '∞'}</td>
                  <td className="text-text-tertiary text-small">{fmtRelative(k.last_used_at)}</td>
                  <td>
                    <div className="flex justify-end gap-1">
                      <button
                        title={k.status === STATUS_ENABLED ? '停用' : '启用'}
                        className={clsx(
                          'btn btn-ghost btn-icon btn-sm',
                          k.status === STATUS_ENABLED ? '' : 'text-warning',
                        )}
                        onClick={() => toggleMut.mutate({ id: k.id, enable: k.status !== STATUS_ENABLED })}
                      >
                        <Power size={16} />
                      </button>
                      <button
                        title="删除"
                        className="btn btn-danger-ghost btn-icon btn-sm"
                        onClick={() => {
                          if (confirm(`确认删除「${k.name}」?`)) removeMut.mutate(k.id);
                        }}
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {showCreate && (
        <CreateKeyDialog
          onClose={() => setShowCreate(false)}
          onSubmit={(body) => createMut.mutate(body)}
          submitting={createMut.isPending}
        />
      )}
    </div>
  );
}

interface CreateKeyDialogProps {
  onClose: () => void;
  onSubmit: (body: {
    name: string;
    scope?: string;
    rpm_limit?: number;
    daily_quota?: number;
    expire_days?: number;
  }) => void;
  submitting: boolean;
}

function CreateKeyDialog({ onClose, onSubmit, submitting }: CreateKeyDialogProps) {
  const [name, setName] = useState('');
  const [scope, setScope] = useState(SCOPE_OPTIONS[0]!.value);
  const [rpm, setRpm] = useState(60);
  const [expireDays, setExpireDays] = useState(0);

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-surface-overlay px-4 backdrop-blur-sm">
      <div className="dialog-surface w-full max-w-md p-6 klein-fade-in">
        <header className="flex items-center justify-between mb-5">
          <h2 className="text-h3 text-text-primary">创建 API KEY</h2>
          <button className="btn btn-ghost btn-icon btn-sm" onClick={onClose} aria-label="关闭">
            <X size={16} />
          </button>
        </header>

        <div className="space-y-4">
          <div className="field">
            <label className="field-label">名称</label>
            <input
              className="input"
              placeholder="例：生产环境 / 本地调试"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={64}
            />
          </div>

          <div className="field">
            <label className="field-label">权限</label>
            <select className="select" value={scope} onChange={(e) => setScope(e.target.value)}>
              {SCOPE_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="field">
              <label className="field-label">RPM 限速</label>
              <input
                type="number"
                className="input"
                value={rpm}
                min={0}
                max={10000}
                onChange={(e) => setRpm(Number(e.target.value) || 0)}
              />
            </div>
            <div className="field">
              <label className="field-label">有效期（天，0=永久）</label>
              <input
                type="number"
                className="input"
                value={expireDays}
                min={0}
                max={3650}
                onChange={(e) => setExpireDays(Number(e.target.value) || 0)}
              />
            </div>
          </div>
        </div>

        <div className="mt-6 flex justify-end gap-2">
          <button className="btn btn-outline btn-md" onClick={onClose}>取消</button>
          <button
            className="btn btn-primary btn-md"
            disabled={!name.trim() || submitting}
            onClick={() =>
              onSubmit({
                name: name.trim(),
                scope,
                rpm_limit: rpm,
                expire_days: expireDays,
              })
            }
          >
            {submitting ? '创建中…' : '创建'}
          </button>
        </div>
      </div>
    </div>
  );
}

function CreatedKeyCard({ data, onClose }: { data: APIKeyCreated; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  return (
    <section className="card-tinted card-section mb-5 klein-fade-in">
      <header className="flex items-center justify-between mb-3">
        <h3 className="text-h4 text-text-primary">KEY「{data.name}」创建成功</h3>
        <button className="btn btn-ghost btn-icon btn-sm" onClick={onClose} aria-label="关闭">
          <X size={16} />
        </button>
      </header>
      <p className="text-small text-text-secondary mb-3">
        请立即复制以下明文，关闭后将无法再次查看。
      </p>
      <div className="flex flex-col sm:flex-row items-stretch gap-2">
        <code className="flex-1 rounded-sm bg-surface-1 border border-border px-4 py-3 font-mono text-body break-all">
          {data.plain}
        </code>
        <button
          className="btn btn-primary btn-lg sm:min-w-[120px]"
          onClick={() => {
            navigator.clipboard.writeText(data.plain).then(() => {
              setCopied(true);
              toast.success('已复制到剪贴板');
              setTimeout(() => setCopied(false), 2000);
            });
          }}
        >
          {copied ? <Check size={16} /> : <Copy size={16} />}
          {copied ? '已复制' : '复制'}
        </button>
      </div>
    </section>
  );
}
