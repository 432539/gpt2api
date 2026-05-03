import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ChevronDown,
  ChevronRight,
  Eye,
  ImageIcon,
  MessageSquare,
  RefreshCw,
  Search,
  Trash2,
  Video,
  X,
} from 'lucide-react';
import { Fragment, useMemo, useState } from 'react';

import { ApiError } from '../../lib/api';
import { fmtPoints, fmtTime } from '../../lib/format';
import { logsApi } from '../../lib/services';
import type { AdminGenerationLogItem, AdminGenerationUpstreamLogItem } from '../../lib/types';
import { toast } from '../../stores/toast';

const pageSize = 20;

function statusInfo(s: number): { label: string; cls: string } {
  switch (s) {
    case 0:
      return { label: '待处理', cls: 'badge badge-outline' };
    case 1:
      return { label: '生成中', cls: 'badge badge-warning' };
    case 2:
      return { label: '成功', cls: 'badge badge-success' };
    case 3:
      return { label: '失败', cls: 'badge badge-danger' };
    case 4:
      return { label: '已退款', cls: 'badge badge-warning' };
    default:
      return { label: String(s), cls: 'badge badge-outline' };
  }
}

function kindInfo(kind: string) {
  if (kind === 'video') return { label: '视频', icon: Video };
  if (kind === 'chat' || kind === 'text') return { label: '文字', icon: MessageSquare };
  return { label: '图片', icon: ImageIcon };
}

function fmtDuration(ms?: number): string {
  if (!ms || ms <= 0) return '-';
  if (ms < 1000) return `${ms}ms`;
  const sec = ms / 1000;
  if (sec < 60) return `${sec.toFixed(1)}s`;
  return `${Math.floor(sec / 60)}m ${Math.round(sec % 60)}s`;
}

function Preview({ row }: { row: AdminGenerationLogItem }) {
  if (!row.preview_url) return <span className="text-text-tertiary">-</span>;
  if (row.kind === 'video') {
    return (
      <a className="btn btn-ghost btn-sm" href={row.preview_url} target="_blank" rel="noreferrer">
        <Video size={14} /> 查看
      </a>
    );
  }
  return (
    <a
      href={row.preview_url}
      target="_blank"
      rel="noreferrer"
      className="block h-10 w-10 overflow-hidden rounded-md border border-border bg-surface-2"
    >
      <img src={row.preview_url} alt="" className="h-full w-full object-cover" />
    </a>
  );
}

export default function LogsPage() {
  const qc = useQueryClient();
  const [keyword, setKeyword] = useState('');
  const [kind, setKind] = useState<'all' | 'image' | 'video' | 'chat'>('all');
  const [status, setStatus] = useState<'all' | '0' | '1' | '2' | '3' | '4'>('all');
  const [page, setPage] = useState(1);
  const [purgeDays, setPurgeDays] = useState('30');
  const [confirmPurge, setConfirmPurge] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [upstreamTask, setUpstreamTask] = useState<AdminGenerationLogItem | null>(null);

  const query = useMemo(
    () => ({
      keyword: keyword.trim() || undefined,
      kind: kind === 'all' ? undefined : kind,
      status: status === 'all' ? undefined : (Number(status) as 0 | 1 | 2 | 3 | 4),
      page,
      page_size: pageSize,
    }),
    [keyword, kind, status, page],
  );

  const list = useQuery({
    queryKey: ['admin', 'logs', 'generations', query],
    queryFn: () => logsApi.generations(query),
  });

  const items = list.data?.list ?? [];
  const total = list.data?.total ?? 0;
  const lastPage = Math.max(1, Math.ceil(total / pageSize));
  const purgeDayNum = Math.max(1, Math.floor(Number(purgeDays) || 0));

  const purge = useMutation({
    mutationFn: () => logsApi.purgeGenerations(purgeDayNum),
    onSuccess: (r) => {
      toast.success(`已删除 ${purgeDayNum} 天前的 ${r.deleted} 条请求日志`);
      setConfirmPurge(false);
      setPage(1);
      qc.invalidateQueries({ queryKey: ['admin', 'logs'] });
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">请求日志</h1>
          <p className="page-subtitle">按任务查看用户、模型、状态和费用；长提示词、错误和上游返回收进详情里。</p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <div className="inline-flex h-11 items-center gap-2 rounded-xl border border-border bg-surface-1 px-2">
            <span className="text-small text-text-tertiary">删除</span>
            <input
              className="input h-8 w-16 rounded-lg px-2 text-center"
              value={purgeDays}
              inputMode="numeric"
              onChange={(e) => setPurgeDays(e.target.value.replace(/\D/g, '').slice(0, 4))}
              aria-label="删除几天前"
            />
            <span className="text-small text-text-tertiary">天前</span>
            <button className="btn btn-danger btn-sm" disabled={purge.isPending || purgeDayNum <= 0} onClick={() => setConfirmPurge(true)}>
              <Trash2 size={14} /> 删除
            </button>
          </div>
          <button className="btn btn-outline btn-md" onClick={() => qc.invalidateQueries({ queryKey: ['admin', 'logs'] })}>
            <RefreshCw size={16} /> 刷新
          </button>
        </div>
      </header>

      <div className="card card-section grid gap-2 !py-2 xl:grid-cols-[minmax(360px,1fr)_auto_auto]">
        <div className="relative">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input
            className="input h-11 pl-8"
            placeholder="搜索用户 / Key / 模型 / 提示词 / task_id"
            value={keyword}
            onChange={(e) => {
              setKeyword(e.target.value);
              setPage(1);
            }}
          />
        </div>
        <div className="tabs h-11">
          {[
            ['all', '全部'],
            ['chat', '文字'],
            ['image', '图片'],
            ['video', '视频'],
          ].map(([k, label]) => (
            <button
              key={k}
              className="tab"
              aria-selected={kind === k}
              onClick={() => {
                setKind(k as typeof kind);
                setPage(1);
              }}
            >
              {label}
            </button>
          ))}
        </div>
        <select
          className="input h-11 w-full xl:w-[132px]"
          value={status}
          onChange={(e) => {
            setStatus(e.target.value as typeof status);
            setPage(1);
          }}
        >
          <option value="all">全部状态</option>
          <option value="0">待处理</option>
          <option value="1">生成中</option>
          <option value="2">成功</option>
          <option value="3">失败</option>
          <option value="4">已退款</option>
        </select>
      </div>

      <div className="card table-wrap overflow-hidden">
        <table className="data-table table-fixed text-small">
          <thead>
            <tr>
              <th className="w-[42px]" />
              <th className="w-[156px]">时间</th>
              <th className="w-[160px]">用户</th>
              <th className="w-[150px]">Key</th>
              <th className="w-[170px]">模型</th>
              <th className="w-[92px]">状态</th>
              <th className="w-[86px]">耗时</th>
              <th className="w-[86px]">费用</th>
              <th className="w-[88px]">预览</th>
              <th className="w-[118px]">上游</th>
            </tr>
          </thead>
          <tbody>
            {list.isLoading && (
              <tr>
                <td colSpan={10} className="py-10 text-center text-text-tertiary">加载中...</td>
              </tr>
            )}
            {!list.isLoading && items.length === 0 && (
              <tr>
                <td colSpan={10} className="py-10 text-center text-text-tertiary">暂无生成记录</td>
              </tr>
            )}
            {items.map((row) => {
              const st = statusInfo(row.status);
              const ki = kindInfo(row.kind);
              const KindIcon = ki.icon;
              const isOpen = expanded === row.task_id;
              return (
                <Fragment key={row.task_id}>
                  <tr className="align-middle">
                    <td>
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title={isOpen ? '收起详情' : '展开详情'}
                        onClick={() => setExpanded(isOpen ? null : row.task_id)}
                      >
                        {isOpen ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                      </button>
                    </td>
                    <td className="whitespace-nowrap">
                      <div>{fmtTime(row.created_at)}</div>
                      <div className="truncate text-tiny text-text-tertiary">{row.task_id}</div>
                    </td>
                    <td>
                      <div className="truncate">{row.user_label}</div>
                      <div className="text-tiny text-text-tertiary">UID {row.user_id}</div>
                    </td>
                    <td className="truncate" title={row.key_label || '-'}>
                      {row.key_label || '-'}
                    </td>
                    <td>
                      <div className="flex min-w-0 items-center gap-1.5">
                        <KindIcon size={14} className="shrink-0 text-text-tertiary" />
                        <span className="truncate" title={row.model_code}>{row.model_code}</span>
                      </div>
                    </td>
                    <td><span className={st.cls}>{st.label}</span></td>
                    <td>{fmtDuration(row.duration_ms)}</td>
                    <td>{fmtPoints(row.cost_points)}</td>
                    <td><Preview row={row} /></td>
                    <td>
                      <button className="btn btn-ghost btn-sm" onClick={() => setUpstreamTask(row)}>
                        <Eye size={14} /> 日志
                      </button>
                    </td>
                  </tr>
                  {isOpen && (
                    <tr>
                      <td colSpan={10} className="bg-surface-2/60 p-0">
                        <div className="grid gap-3 p-4 lg:grid-cols-[1fr_1fr]">
                          <DetailBlock title="提示词" value={row.prompt || '-'} />
                          <DetailBlock title="错误信息" value={row.error || '-'} danger={Boolean(row.error)} />
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between text-small text-text-tertiary">
        <span>共 {total} 条记录</span>
        <div className="inline-flex items-center gap-2">
          <button className="btn btn-outline btn-sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</button>
          <span>{page} / {lastPage}</span>
          <button className="btn btn-outline btn-sm" disabled={page >= lastPage} onClick={() => setPage((p) => Math.min(lastPage, p + 1))}>下一页</button>
        </div>
      </div>

      {upstreamTask && <UpstreamDialog task={upstreamTask} onClose={() => setUpstreamTask(null)} />}
      {confirmPurge && (
        <ConfirmDialog
          days={purgeDayNum}
          loading={purge.isPending}
          onClose={() => setConfirmPurge(false)}
          onConfirm={() => purge.mutate()}
        />
      )}
    </div>
  );
}

function DetailBlock({ title, value, danger }: { title: string; value: string; danger?: boolean }) {
  return (
    <div className="rounded-xl border border-border bg-surface-1 p-3">
      <div className="mb-2 text-tiny text-text-tertiary">{title}</div>
      <div className={`max-h-36 overflow-auto whitespace-pre-wrap break-words text-small leading-relaxed ${danger ? 'text-danger' : 'text-text-secondary'}`}>
        {value}
      </div>
    </div>
  );
}

function UpstreamDialog({ task, onClose }: { task: AdminGenerationLogItem; onClose: () => void }) {
  const q = useQuery({
    queryKey: ['admin', 'logs', 'generations', task.task_id, 'upstream'],
    queryFn: () => logsApi.generationUpstream(task.task_id),
  });
  const rows = q.data ?? [];
  return (
    <div className="fixed inset-0 z-[80] grid place-items-center bg-black/40 p-4 backdrop-blur-sm">
      <div className="dialog-surface klein-fade-in max-h-[86vh] w-full max-w-5xl overflow-hidden">
        <header className="modal-header">
          <div>
            <h2 className="text-h4">上游日志</h2>
            <p className="text-small text-text-tertiary">{task.task_id} · {task.model_code}</p>
          </div>
          <button className="btn btn-ghost btn-sm" onClick={onClose}><X size={16} /></button>
        </header>
        <div className="modal-body max-h-[70vh] space-y-3 overflow-auto">
          {q.isLoading && <div className="py-10 text-center text-text-tertiary">加载中...</div>}
          {!q.isLoading && rows.length === 0 && <div className="py-10 text-center text-text-tertiary">暂无上游日志，新任务会自动记录。</div>}
          {rows.map((row) => <UpstreamRow key={row.id} row={row} />)}
        </div>
      </div>
    </div>
  );
}

function UpstreamRow({ row }: { row: AdminGenerationUpstreamLogItem }) {
  return (
    <section className="rounded-xl border border-border bg-surface-1 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="badge badge-outline">{row.stage}</span>
          {row.method && <span className="text-small text-text-tertiary">{row.method}</span>}
          {row.status_code > 0 && <span className="text-small text-text-tertiary">HTTP {row.status_code}</span>}
          {row.duration_ms > 0 && <span className="text-small text-text-tertiary">{fmtDuration(row.duration_ms)}</span>}
        </div>
        <span className="text-tiny text-text-tertiary">{fmtTime(row.created_at)}</span>
      </div>
      {row.url && <div className="mt-2 break-all text-tiny text-text-tertiary">{row.url}</div>}
      <LogBlock title="请求" value={row.request_excerpt} />
      <LogBlock title="响应" value={row.response_excerpt} />
      <LogBlock title="错误" value={row.error} danger />
      <LogBlock title="附加信息" value={prettyMeta(row.meta)} />
    </section>
  );
}

function LogBlock({ title, value, danger }: { title: string; value?: string; danger?: boolean }) {
  if (!value) return null;
  return (
    <div className="mt-3">
      <div className="mb-1 text-tiny text-text-tertiary">{title}</div>
      <pre className={`max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-border bg-surface-2 p-3 text-tiny ${danger ? 'text-danger' : 'text-text-secondary'}`}>
        {value}
      </pre>
    </div>
  );
}

function ConfirmDialog({ days, loading, onClose, onConfirm }: { days: number; loading: boolean; onClose: () => void; onConfirm: () => void }) {
  return (
    <div className="fixed inset-0 z-[90] grid place-items-center bg-black/40 p-4 backdrop-blur-sm">
      <div className="dialog-surface klein-fade-in w-full max-w-md p-6">
        <div className="mb-4 flex items-start gap-3">
          <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-danger/10 text-danger">
            <Trash2 size={18} />
          </div>
          <div>
            <h2 className="text-h4">删除请求日志</h2>
            <p className="mt-1 text-small text-text-secondary">确定删除 {days} 天前的请求日志吗？该操作不可恢复。</p>
          </div>
        </div>
        <div className="flex justify-end gap-2">
          <button className="btn btn-outline btn-md" disabled={loading} onClick={onClose}>取消</button>
          <button className="btn btn-danger btn-md" disabled={loading} onClick={onConfirm}>
            {loading ? '删除中...' : '确认删除'}
          </button>
        </div>
      </div>
    </div>
  );
}

function prettyMeta(v?: string) {
  if (!v) return '';
  try {
    return JSON.stringify(JSON.parse(v), null, 2);
  } catch {
    return v;
  }
}
