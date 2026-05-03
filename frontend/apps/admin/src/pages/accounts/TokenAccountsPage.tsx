import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Plus, Upload, RefreshCw, Trash2, Power, Activity, RotateCw, CheckCircle2, XCircle, Clock,
  ChevronDown, ChevronUp, AlertCircle, Pencil,
  ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight,
} from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';

import { ApiError } from '../../lib/api';
import { fmtNumber, fmtRelative, fmtTime, statusLabel } from '../../lib/format';
import { accountsApi, proxiesApi } from '../../lib/services';
import type {
  AccountBatchImportBody,
  AccountCreateBody,
  AccountItem,
  AccountPurgeBody,
  AccountUpdateBody,
  ProxyItem,
  Sub2APIAccountItem,
} from '../../lib/types';
import { toast } from '../../stores/toast';

/** 把用户可能漏 scheme 的 host 自动补成 https://；空字符串保持空 */
function normalizeBaseURL(s?: string): string | undefined {
  const v = (s || '').trim();
  if (!v) return undefined;
  if (/^https?:\/\//i.test(v)) return v;
  return `https://${v}`;
}

/** 默认 auth_type：GPT 走 OAuth (AT/RT/ST)；GROK 走 SSO Token。 */
function defaultAuthType(provider: 'gpt' | 'grok'): 'api_key' | 'oauth' | 'cookie' {
  return provider === 'gpt' ? 'oauth' : 'cookie';
}

const TONE_CLS: Record<'ok' | 'warn' | 'err' | 'mute', string> = {
  ok: 'badge badge-success',
  warn: 'badge badge-warning',
  err: 'badge badge-danger',
  mute: 'badge',
};

function testLabel(s?: number): { label: string; cls: string; icon: typeof CheckCircle2 } {
  switch (s) {
    case 1: return { label: 'OK',   cls: 'text-success', icon: CheckCircle2 };
    case 2: return { label: 'FAIL', cls: 'text-danger',  icon: XCircle };
    default: return { label: '未测', cls: 'text-text-tertiary', icon: Clock };
  }
}

function expireState(expSec?: number): { label: string; detail: string; cls: string } {
  if (!expSec) return { label: '未设置', detail: '未设置到期时间', cls: 'text-text-tertiary' };
  const expIn = expSec - Date.now() / 1000;
  if (expIn <= 0) return { label: '已过期', detail: fmtTime(expSec), cls: 'text-danger' };
  if (expIn < 3600) return { label: `${Math.max(1, Math.floor(expIn / 60))} 分钟`, detail: fmtTime(expSec), cls: 'text-warning' };
  if (expIn < 86400) return { label: `${Math.floor(expIn / 3600)} 小时`, detail: fmtTime(expSec), cls: 'text-warning' };
  return { label: `${Math.floor(expIn / 86400)} 天`, detail: fmtTime(expSec), cls: 'text-text-secondary' };
}

/** 调度状态 + 最近错误/连通结果，用于列表「状态」列（避免启用仍显示「正常」）。 */
function accountRowStatus(r: AccountItem): { label: string; tone: 'ok' | 'warn' | 'err' | 'mute' } {
  const base = statusLabel(r.status);
  if (r.status !== 1) {
    return { label: base.label, tone: base.tone };
  }
  const le = (r.last_error || '').trim();
  const te = (r.last_test_error || '').trim();
  const testFail = r.last_test_status === 2;
  if (le || testFail || te) {
    return { label: '异常', tone: 'err' };
  }
  return { label: base.label, tone: base.tone };
}

export default function TokenAccountsPage() {
  const qc = useQueryClient();

  const [provider, setProvider] = useState<'all' | 'gpt' | 'grok'>('all');
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<number>(10);
  const PAGE_SIZE_OPTIONS = [10, 20, 50, 100, 1000];

  const [openCreate, setOpenCreate] = useState(false);
  const [openImport, setOpenImport] = useState(false);
  const [editTarget, setEditTarget] = useState<AccountItem | null>(null);

  const query = useMemo(
    () => ({
      provider: provider === 'all' ? undefined : provider,
      keyword: keyword || undefined,
      page,
      page_size: pageSize,
    }),
    [provider, keyword, page, pageSize],
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

  const testMut = useMutation({
    mutationFn: (id: number) => accountsApi.test(id),
    onSuccess: (r) => {
      refresh();
      if (r.ok) {
        toast.success(`连通正常 · ${r.latency_ms}ms`);
      } else {
        toast.error(`不可用：${r.error || '未知错误'}`);
      }
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const refreshOAuthMut = useMutation({
    mutationFn: (id: number) => accountsApi.refresh(id),
    onSuccess: (r) => {
      refresh();
      const ttl = r.expires_in ? `，有效期 ${Math.floor(r.expires_in / 3600)}h` : '';
      toast.success(`已刷新 access_token${ttl}`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const batchRefresh = useMutation({
    mutationFn: async (p: 'gpt' | 'grok' | '') => {
      let page = 1;
      let refreshed = 0;
      const failed_ids: number[] = [];
      let total = 0;
      const batchPageSize = Math.min(Math.max(pageSize, 1), 1000);
      for (;;) {
        const r = await accountsApi.batchRefresh(p || undefined, page, batchPageSize);
        refreshed += r.refreshed;
        failed_ids.push(...r.failed_ids);
        total = r.total || total;
        if (r.has_more && r.next_page) {
          page = r.next_page;
          continue;
        }
        if (total > 0 && page * batchPageSize < total) {
          page += 1;
          continue;
        }
        break;
      }
      return { refreshed, failed_ids };
    },
    onSuccess: (r) => {
      refresh();
      toast.success(`已刷新 ${r.refreshed} 个 OAuth 账号${r.failed_ids.length ? `，失败 ${r.failed_ids.length}` : ''}`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const batchProbe = useMutation({
    mutationFn: async (p: 'gpt' | 'grok' | '') => {
      let page = 1;
      let probed = 0;
      const failed_ids: number[] = [];
      const batchPageSize = Math.min(Math.max(pageSize, 1), 1000);
      let total = 0;
      for (;;) {
        const r = await accountsApi.batchProbe(p || undefined, page, batchPageSize);
        probed += r.probed;
        failed_ids.push(...r.failed_ids);
        total = r.total || total;
        if (r.has_more && r.next_page) {
          page = r.next_page;
          continue;
        }
        if (total > 0 && page * batchPageSize < total) {
          page += 1;
          continue;
        }
        break;
      }
      return { probed, failed_ids };
    },
    onSuccess: (r) => {
      refresh();
      toast.success(`已检测 ${r.probed} 个账号用量${r.failed_ids.length ? `，失败 ${r.failed_ids.length} 个` : ''}`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const total = list.data?.total ?? 0;
  const items: AccountItem[] = list.data?.list ?? [];
  const lastPage = Math.max(1, Math.ceil(total / pageSize));

  const [selected, setSelected] = useState<Set<number>>(new Set());

  useEffect(() => {
    setSelected(new Set());
  }, [provider, keyword]);

  const toggleSelect = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectPage = () => {
    const pageIds = items.map((r) => r.id);
    const allOn = pageIds.length > 0 && pageIds.every((id) => selected.has(id));
    setSelected((prev) => {
      const next = new Set(prev);
      if (allOn) {
        pageIds.forEach((id) => next.delete(id));
      } else {
        pageIds.forEach((id) => next.add(id));
      }
      return next;
    });
  };

  const batchDeleteMut = useMutation({
    mutationFn: (ids: number[]) => accountsApi.batchDelete(ids),
    onSuccess: (r) => {
      refresh();
      setSelected(new Set());
      toast.success(`已删除 ${r.deleted} 条`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const purgeMut = useMutation({
    mutationFn: (b: AccountPurgeBody) => accountsApi.purge(b),
    onSuccess: (r) => {
      refresh();
      setSelected(new Set());
      toast.success(`已清理 ${r.deleted} 条`);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const purgeProvider = provider === 'all' ? undefined : provider;

  const pageIds = items.map((r) => r.id);
  const pageAllSelected = pageIds.length > 0 && pageIds.every((id) => selected.has(id));
  const headerCbRef = useRef<HTMLInputElement | null>(null);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [purgeAllOpen, setPurgeAllOpen] = useState(false);
  const bulkWrapRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    if (!bulkOpen) return;
    const onDocClick = (e: MouseEvent) => {
      if (!bulkWrapRef.current) return;
      if (!bulkWrapRef.current.contains(e.target as Node)) setBulkOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setBulkOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onKey);
    };
  }, [bulkOpen]);

  useEffect(() => {
    const el = headerCbRef.current;
    if (!el) return;
    const some = pageIds.some((id) => selected.has(id));
    el.indeterminate = some && !pageAllSelected;
  }, [pageIds, selected, pageAllSelected]);

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">Token 管理</h1>
          <p className="page-subtitle">GPT / GROK 账号池</p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <button className="btn btn-outline btn-sm" onClick={refresh} title="刷新列表">
            <RefreshCw size={14} /> 刷新
          </button>
          <button
            className="btn btn-outline btn-sm"
            onClick={() => batchRefresh.mutate(provider === 'all' ? '' : provider)}
            disabled={batchRefresh.isPending}
            title="按当前 Tab 批量刷新 OAuth"
          >
            <RotateCw size={14} className={batchRefresh.isPending ? 'animate-spin' : ''} />
            {batchRefresh.isPending ? '刷新中…' : '批量刷新OAuth'}
          </button>
          <button
            className="btn btn-outline btn-sm"
            onClick={() => batchProbe.mutate(provider === 'all' ? '' : provider)}
            disabled={batchProbe.isPending}
            title="按当前 Tab 批量检测账号用量/额度"
          >
            <Activity size={14} className={batchProbe.isPending ? 'animate-pulse' : ''} />
            {batchProbe.isPending ? '检测中…' : '批量检测用量'}
          </button>
          <button className="btn btn-outline btn-sm" onClick={() => setOpenImport(true)}>
            <Upload size={14} /> 导入
          </button>
          <div ref={bulkWrapRef} className="relative">
            <button
              type="button"
              className="btn btn-outline btn-sm gap-1"
              disabled={batchDeleteMut.isPending || purgeMut.isPending}
              aria-haspopup="menu"
              aria-expanded={bulkOpen}
              onClick={() => setBulkOpen((v) => !v)}
            >
              <Trash2 size={14} />
              删除
              <ChevronDown
                size={14}
                className={`opacity-60 transition-transform ${bulkOpen ? 'rotate-180' : ''}`}
              />
            </button>
            {bulkOpen && (
              <div
                role="menu"
                className="card card-elevated absolute right-0 mt-2 z-[90] w-[18rem] overflow-hidden p-1.5 shadow-xl klein-fade-in"
              >
                <div className="px-2.5 py-2">
                  <div className="text-small font-semibold text-text-primary">批量删除</div>
                  <div className="text-tiny text-text-tertiary mt-0.5">
                    作用范围：{provider === 'all' ? '全部账号' : provider.toUpperCase()}
                  </div>
                </div>
                <button
                  type="button"
                  role="menuitem"
                  className="btn btn-ghost btn-sm w-full justify-between gap-2"
                  disabled={selected.size === 0 || batchDeleteMut.isPending}
                  onClick={() => {
                    setBulkOpen(false);
                    if (selected.size === 0) return;
                    if (!confirm(`软删除选中的 ${selected.size} 条？`)) return;
                    batchDeleteMut.mutate([...selected]);
                  }}
                >
                  <span className="inline-flex items-center gap-2">
                    <CheckCircle2 size={14} className="text-text-secondary" />
                    删除选中
                  </span>
                  <span className="badge badge-outline">{selected.size}</span>
                </button>
                <button
                  type="button"
                  role="menuitem"
                  className="btn btn-ghost btn-sm w-full justify-start gap-2"
                  disabled={purgeMut.isPending}
                  onClick={() => {
                    setBulkOpen(false);
                    if (!confirm('删除禁用、熔断或测试失败的账号？')) return;
                    purgeMut.mutate({ scope: 'invalid', provider: purgeProvider });
                  }}
                >
                  <XCircle size={14} className="text-danger" />
                  删除错误
                </button>
                <button
                  type="button"
                  role="menuitem"
                  className="btn btn-ghost btn-sm w-full justify-start gap-2"
                  disabled={purgeMut.isPending}
                  onClick={() => {
                    setBulkOpen(false);
                    if (!confirm('删除已检测且剩余额度为 0 的账号？未检测账号不会被删除。')) return;
                    purgeMut.mutate({ scope: 'zero_quota', provider: purgeProvider });
                  }}
                >
                  <AlertCircle size={14} className="text-warning" />
                  删除0额度
                </button>
                <div className="my-1 h-px bg-border" />
                <button
                  type="button"
                  role="menuitem"
                  className="btn btn-danger-ghost btn-sm w-full justify-start gap-2"
                  disabled={purgeMut.isPending}
                  onClick={() => {
                    setBulkOpen(false);
                    setPurgeAllOpen(true);
                  }}
                >
                  <Trash2 size={14} />
                  删除全部
                </button>
              </div>
            )}
          </div>
          <button className="btn btn-primary btn-sm" onClick={() => setOpenCreate(true)}>
            <Plus size={16} /> 新增
          </button>
        </div>
      </header>

      {/* 筛选 */}
      <div className="card card-section flex flex-wrap items-center gap-2 !py-2">
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
          className="input input-sm flex-1 min-w-[160px]"
          placeholder="名称 / 备注"
          value={keyword}
          onChange={(e) => { setKeyword(e.target.value); setPage(1); }}
        />
        <span className="text-tiny text-text-tertiary whitespace-nowrap">
          共 <span className="text-text-secondary tabular-nums font-medium">{fmtNumber(total)}</span> 条
        </span>
      </div>

      {/* 表格 */}
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
              <th>Provider</th>
              <th>状态</th>
              <th>凭证 / 最近测试</th>
              <th>用量</th>
              <th>到期时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {list.isLoading && (
              <tr>
                <td colSpan={8} className="text-center text-text-tertiary text-small py-10">加载中…</td>
              </tr>
            )}
            {!list.isLoading && items.length === 0 && (
              <tr>
                <td colSpan={8}>
                  <div className="empty-state">
                    <p className="empty-state-title">暂无账号</p>
                    <p className="empty-state-desc">点击右上角【新增账号】或【批量导入】开始。</p>
                  </div>
                </td>
              </tr>
            )}
            {items.map((r) => {
              const s = accountRowStatus(r);
              const enabled = r.status === 1;
              const isOAuth = r.auth_type === 'oauth';
              const t = testLabel(r.last_test_status);
              const TestIcon = t.icon;
              const exp = expireState(r.access_token_expire_at);
              const lastErr = (r.last_error || '').trim();
              const testErr = (r.last_test_error || '').trim();
              const statusErrTip = [lastErr, testErr].filter(Boolean).join('\n\n');
              const atNeedsAttention =
                isOAuth && (!r.has_access_token || r.last_test_status === 2 || !!testErr);
              return (
                <tr key={r.id}>
                  <td className="w-10">
                    <input
                      type="checkbox"
                      className="rounded border-border"
                      checked={selected.has(r.id)}
                      onChange={() => toggleSelect(r.id)}
                      aria-label={`选择 ${r.name}`}
                    />
                  </td>
                  <td className="font-medium text-text-primary">
                    {r.name}
                    {r.remark && (
                      <span className="block text-small text-text-tertiary mt-0.5">{r.remark}</span>
                    )}
                  </td>
                  <td className="uppercase text-klein-500 font-semibold">{r.provider}</td>
                  <td className="whitespace-nowrap">
                    <span className={TONE_CLS[s.tone]}>{s.label}</span>
                    {!!statusErrTip && (
                      <span
                        className="inline-flex align-middle ml-1 text-danger"
                        title={statusErrTip}
                        aria-label={statusErrTip}
                      >
                        <AlertCircle size={14} strokeWidth={2} />
                      </span>
                    )}
                  </td>
                  <td className="text-small">
                    {isOAuth ? (
                      <div className="flex flex-col gap-1">
                        <div className="inline-flex gap-1 items-center">
                          <span
                            className={`badge text-tiny ${r.has_refresh_token ? 'badge-success' : 'badge-warning'}`}
                            title="refresh_token 是否已存"
                          >
                            RT {r.has_refresh_token ? '✓' : '✗'}
                          </span>
                          <span
                            className={`badge text-tiny ${
                              r.has_access_token ? 'badge-success' : atNeedsAttention ? 'badge-warning' : ''
                            }`}
                            title="access_token 是否已取得"
                          >
                            AT {r.has_access_token ? '✓' : '∅'}
                          </span>
                        </div>
                        <div className={`inline-flex items-center gap-1 flex-wrap ${t.cls}`}>
                          <TestIcon size={12} />
                          <span className="text-tiny">
                            {t.label}
                            {r.last_test_latency_ms ? ` · ${r.last_test_latency_ms}ms` : ''}
                          </span>
                          {r.last_test_at && (
                            <span className="text-tiny text-text-tertiary">{fmtRelative(r.last_test_at)}</span>
                          )}
                          {testErr && (
                            <span
                              className="inline-flex text-danger"
                              title={r.last_test_error}
                              aria-label={r.last_test_error}
                            >
                              <AlertCircle size={12} strokeWidth={2} />
                            </span>
                          )}
                        </div>
                      </div>
                    ) : (
                      <div className={`inline-flex items-center gap-1 flex-wrap ${t.cls}`}>
                        <TestIcon size={12} />
                        <span className="text-tiny">
                          {t.label}
                          {r.last_test_latency_ms ? ` · ${r.last_test_latency_ms}ms` : ''}
                        </span>
                        {r.last_test_at && (
                          <span className="text-tiny text-text-tertiary">{fmtRelative(r.last_test_at)}</span>
                        )}
                        {testErr && (
                          <span
                            className="inline-flex text-danger"
                            title={r.last_test_error}
                            aria-label={r.last_test_error}
                          >
                            <AlertCircle size={12} strokeWidth={2} />
                          </span>
                        )}
                      </div>
                    )}
                  </td>
                  <td className="text-small">
                    {r.image_quota_total ? (
                      <span className="text-text-primary">
                        已用 {fmtNumber(Math.max(0, r.image_quota_total - (r.image_quota_remaining ?? 0)))} / {fmtNumber(r.image_quota_total)}
                      </span>
                    ) : typeof r.image_quota_remaining === 'number' ? (
                      <span className="text-text-secondary">剩余 {fmtNumber(r.image_quota_remaining)} / 总额未知</span>
                    ) : (
                      <span className="text-text-tertiary">未检测</span>
                    )}
                  </td>
                  <td className="text-small">
                    <div className="flex flex-col">
                      <span className={exp.cls}>{exp.label}</span>
                      <span className="text-tiny text-text-tertiary">{exp.detail}</span>
                    </div>
                  </td>
                  <td>
                    <div className="inline-flex gap-1">
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title="测试连通"
                        onClick={() => testMut.mutate(r.id)}
                        disabled={testMut.isPending && testMut.variables === r.id}
                      >
                        <Activity
                          size={14}
                          className={
                            testMut.isPending && testMut.variables === r.id
                              ? 'animate-pulse text-klein-500'
                              : 'text-text-secondary'
                          }
                        />
                      </button>
                      {isOAuth && (
                        <button
                          className="btn btn-ghost btn-icon btn-sm"
                          title="刷新 access_token"
                          onClick={() => refreshOAuthMut.mutate(r.id)}
                          disabled={refreshOAuthMut.isPending && refreshOAuthMut.variables === r.id}
                        >
                          <RotateCw
                            size={14}
                            className={
                              refreshOAuthMut.isPending && refreshOAuthMut.variables === r.id
                                ? 'animate-spin text-klein-500'
                                : 'text-text-secondary'
                            }
                          />
                        </button>
                      )}
                      <button
                        className="btn btn-ghost btn-icon btn-sm"
                        title="编辑"
                        onClick={() => setEditTarget(r)}
                      >
                        <Pencil size={14} className="text-text-secondary" />
                      </button>
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

      {/* 分页栏 */}
      <div className="card card-section flex flex-wrap items-center gap-3 !py-2">
        <div className="flex items-center gap-2 text-small text-text-secondary">
          <span className="text-text-tertiary">每页</span>
          <div className="relative">
            <select
              className="select select-sm pr-7 min-w-[5rem] tabular-nums"
              value={pageSize}
              onChange={(e) => {
                const n = Number(e.target.value) || 20;
                setPageSize(n);
                setPage(1);
              }}
              aria-label="每页条数"
            >
              {PAGE_SIZE_OPTIONS.map((n) => (
                <option key={n} value={n}>{n}</option>
              ))}
            </select>
          </div>
          <span className="text-text-tertiary">条</span>
        </div>

        <div className="text-small text-text-tertiary tabular-nums">
          {total === 0
            ? '0'
            : `${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, total)} / ${fmtNumber(total)}`}
        </div>

        <div className="flex items-center gap-1 ml-auto">
          <button
            type="button"
            className="btn btn-outline btn-icon btn-sm"
            disabled={page <= 1}
            onClick={() => setPage(1)}
            title="第一页"
          >
            <ChevronsLeft size={14} />
          </button>
          <button
            type="button"
            className="btn btn-outline btn-icon btn-sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            title="上一页"
          >
            <ChevronLeft size={14} />
          </button>
          <span className="px-2 text-small text-text-secondary tabular-nums min-w-[3.5rem] text-center">
            <span className="font-medium text-text-primary">{page}</span>
            <span className="text-text-tertiary"> / {lastPage}</span>
          </span>
          <button
            type="button"
            className="btn btn-outline btn-icon btn-sm"
            disabled={page >= lastPage}
            onClick={() => setPage((p) => Math.min(lastPage, p + 1))}
            title="下一页"
          >
            <ChevronRight size={14} />
          </button>
          <button
            type="button"
            className="btn btn-outline btn-icon btn-sm"
            disabled={page >= lastPage}
            onClick={() => setPage(lastPage)}
            title="末页"
          >
            <ChevronsRight size={14} />
          </button>
        </div>
      </div>

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
      {editTarget && (
        <EditDialog
          item={editTarget}
          onClose={() => setEditTarget(null)}
          onSuccess={() => {
            setEditTarget(null);
            refresh();
          }}
        />
      )}
      {purgeAllOpen && (
        <PurgeAllDialog
          provider={purgeProvider}
          loading={purgeMut.isPending}
          onClose={() => setPurgeAllOpen(false)}
          onConfirm={() => {
            purgeMut.mutate(
              {
                scope: 'all',
                provider: purgeProvider,
                confirm: 'DELETE_ALL_ACCOUNTS',
              },
              { onSuccess: () => setPurgeAllOpen(false) },
            );
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
    auth_type: 'oauth',
    access_token: '',
    refresh_token: '',
    session_token: '',
    client_id: '',
    credential: '',
    base_url: '',
    proxy_id: undefined,
    weight: 10,
    rpm_limit: 0,
    tpm_limit: 0,
    daily_quota: 0,
    monthly_quota: 0,
    remark: '',
  });
  const [showAdvanced, setShowAdvanced] = useState(false);

  const isOAuth = body.auth_type === 'oauth';

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
          const name = body.name.trim();
          if (!name) {
            toast.error('请填写名称');
            return;
          }
          const at = (body.access_token || '').trim();
          const rt = (body.refresh_token || '').trim();
          const st = (body.session_token || '').trim();
          const cid = (body.client_id || '').trim();
          const cred = (body.credential || '').trim();
          if (isOAuth) {
            if (!at && !rt) {
              toast.error('请至少填写 Access Token 或 Refresh Token');
              return;
            }
          } else if (!cred) {
            toast.error(body.auth_type === 'cookie' ? '请填写 Grok Token' : '请填写 API Key');
            return;
          }
          const payload: AccountCreateBody = {
            provider: body.provider,
            name,
            auth_type: body.auth_type,
            base_url: normalizeBaseURL(body.base_url),
            proxy_id: body.proxy_id && body.proxy_id > 0 ? body.proxy_id : undefined,
            weight: body.weight ?? 10,
            rpm_limit: body.rpm_limit && body.rpm_limit > 0 ? body.rpm_limit : undefined,
            tpm_limit: body.tpm_limit && body.tpm_limit > 0 ? body.tpm_limit : undefined,
            daily_quota: body.daily_quota && body.daily_quota > 0 ? body.daily_quota : undefined,
            monthly_quota:
              body.monthly_quota && body.monthly_quota > 0 ? body.monthly_quota : undefined,
            remark: body.remark?.trim() || undefined,
          };
          if (isOAuth) {
            if (at) payload.access_token = at;
            if (rt) payload.refresh_token = rt;
            if (st) payload.session_token = st;
            if (cid) payload.client_id = cid;
          } else {
            payload.credential = cred;
          }
          m.mutate(payload);
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Provider">
            <select
              className="select"
              value={body.provider}
              onChange={(e) => {
                const p = e.target.value as 'gpt' | 'grok';
                const at = defaultAuthType(p);
                setBody((s) => ({
                  ...s,
                  provider: p,
                  auth_type: at,
                  credential: at !== 'oauth' ? s.credential : '',
                  access_token: at === 'oauth' ? s.access_token : '',
                  refresh_token: at === 'oauth' ? s.refresh_token : '',
                  session_token: at === 'oauth' ? s.session_token : '',
                  client_id: at === 'oauth' ? s.client_id : '',
                }));
              }}
            >
              <option value="gpt">GPT (生图)</option>
              <option value="grok">GROK (生视频)</option>
            </select>
          </Field>
          <Field label="名称 / 标签">
            <input
              className="input"
              placeholder={body.provider === 'gpt' ? 'GPT-Acc-001' : 'GROK-Acc-001'}
              value={body.name}
              onChange={(e) => setBody((s) => ({ ...s, name: e.target.value }))}
            />
          </Field>
        </div>

        {isOAuth ? (
          <div className="space-y-3">
            <Field label="Access Token" hint="必填或与 Refresh Token 至少一项">
              <textarea
                className="textarea font-mono text-small min-h-[64px]"
                placeholder="eyJhbGc..."
                value={body.access_token || ''}
                onChange={(e) => setBody((s) => ({ ...s, access_token: e.target.value }))}
              />
            </Field>
            <Field label="Refresh Token" hint="过期后自动刷新 Access Token">
              <textarea
                className="textarea font-mono text-small min-h-[64px]"
                placeholder="可选；建议与 AT 同时提供"
                value={body.refresh_token || ''}
                onChange={(e) => setBody((s) => ({ ...s, refresh_token: e.target.value }))}
              />
            </Field>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Field label="Session Token">
                <input
                  className="input font-mono text-small"
                  placeholder="可选"
                  value={body.session_token || ''}
                  onChange={(e) => setBody((s) => ({ ...s, session_token: e.target.value }))}
                />
              </Field>
              <Field label="Client ID">
                <input
                  className="input font-mono text-small"
                  placeholder="留空使用系统默认"
                  value={body.client_id || ''}
                  onChange={(e) => setBody((s) => ({ ...s, client_id: e.target.value }))}
                />
              </Field>
            </div>
          </div>
        ) : (
          <Field label={body.auth_type === 'cookie' ? 'Grok Token' : 'API Key'} hint="保存前 AES-256-GCM 加密落库">
            <textarea
              className="textarea font-mono text-small min-h-[80px]"
              placeholder={body.auth_type === 'cookie' ? 'sso=... 或直接粘贴 SSO token' : 'xai-xxxxxxxxxxxxxxxxxxxxxxxx'}
              value={body.credential || ''}
              onChange={(e) => setBody((s) => ({ ...s, credential: e.target.value }))}
            />
          </Field>
        )}

        <div className="grid grid-cols-2 gap-3">
          <Field label="权重" hint="加权轮询时的相对权重，默认 10，范围 1–1000">
            <input
              type="number"
              className="input"
              min={1}
              max={1000}
              value={body.weight ?? 10}
              onChange={(e) => setBody((s) => ({ ...s, weight: Number(e.target.value) || 10 }))}
            />
          </Field>
          <Field label="备注（可选）">
            <input
              className="input"
              value={body.remark || ''}
              onChange={(e) => setBody((s) => ({ ...s, remark: e.target.value }))}
            />
          </Field>
        </div>

        {/* 高级：限速 / 配额 */}
        <button
          type="button"
          className="btn btn-ghost btn-sm gap-1"
          onClick={() => setShowAdvanced((v) => !v)}
        >
          {showAdvanced ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          高级（限速 / 配额，可选）
        </button>
        {showAdvanced && (
          <div className="card card-flat p-3 space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <Field label="RPM 限速" hint="每分钟最大请求数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.rpm_limit ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({ ...s, rpm_limit: Math.max(0, Number(e.target.value) || 0) }))
                  }
                />
              </Field>
              <Field label="TPM 限速" hint="每分钟最大 token 数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.tpm_limit ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({ ...s, tpm_limit: Math.max(0, Number(e.target.value) || 0) }))
                  }
                />
              </Field>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="每日配额" hint="单日最大调用次数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.daily_quota ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({ ...s, daily_quota: Math.max(0, Number(e.target.value) || 0) }))
                  }
                />
              </Field>
              <Field label="每月配额" hint="单月最大调用次数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.monthly_quota ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({
                      ...s,
                      monthly_quota: Math.max(0, Number(e.target.value) || 0),
                    }))
                  }
                />
              </Field>
            </div>
          </div>
        )}

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

/** 解析 sub2api 导出的 account JSON（根级含 accounts[]） */
function parseSub2ExportJson(raw: string): Sub2APIAccountItem[] {
  let j: unknown;
  try {
    j = JSON.parse(raw);
  } catch {
    throw new Error('文件不是合法 JSON');
  }
  if (typeof j !== 'object' || j === null) {
    throw new Error('JSON 根须为对象');
  }
  const accounts = (j as { accounts?: unknown }).accounts;
  if (!Array.isArray(accounts)) {
    throw new Error('请使用 sub2api / Codex 导出格式：根对象须含 accounts 数组');
  }
  return accounts as Sub2APIAccountItem[];
}

// ============== Import Dialog ==============
function ImportDialog({ onClose, onSuccess }: { onClose: () => void; onSuccess: () => void }) {
  const [importMode, setImportMode] = useState<'lines' | 'sub2api'>('lines');
  const [body, setBody] = useState<AccountBatchImportBody>({
    provider: 'gpt',
    auth_type: 'oauth',
    base_url: '',
    proxy_id: undefined,
    weight: 10,
    text: '',
  });
  const [sub2Accounts, setSub2Accounts] = useState<Sub2APIAccountItem[] | null>(null);
  const [sub2FileLabel, setSub2FileLabel] = useState('');
  const [sub2ChunkSize, setSub2ChunkSize] = useState(300);
  const [sub2Busy, setSub2Busy] = useState(false);

  const proxiesQ = useQuery({
    queryKey: ['admin', 'proxies', 'select'],
    queryFn: () => proxiesApi.list({ status: 1, page_size: 200 }),
    staleTime: 30_000,
  });
  const proxies: ProxyItem[] = proxiesQ.data?.list ?? [];

  const linePlaceholder = useMemo(() => {
    switch (body.auth_type) {
      case 'oauth':
        return '每行 refresh_token，或 name@@token';
      case 'cookie':
        return '每行一个 Grok SSO token，或 name@@sso-token';
      case 'api_key':
      default:
        return 'sk-… 或 name@@sk-… 或 key@https://…';
    }
  }, [body.auth_type]);

  const m = useMutation({
    mutationFn: (b: AccountBatchImportBody) => accountsApi.batchImport(b),
    onSuccess: (r) => {
      const sk = r.skipped > 0 ? `，跳过 ${r.skipped} 条` : '';
      toast.success(`成功导入 ${r.imported} 条${sk}`);
      onSuccess();
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const runSub2Import = async () => {
    if (!sub2Accounts?.length) {
      toast.error('请先选择合法的 sub2api JSON 文件');
      return;
    }
    const chunk = Math.min(500, Math.max(50, sub2ChunkSize));
    setSub2Busy(true);
    let totalImported = 0;
    let totalSkipped = 0;
    try {
      for (let i = 0; i < sub2Accounts.length; i += chunk) {
        const slice = sub2Accounts.slice(i, i + chunk);
        const r = await accountsApi.batchImport({
          format: 'sub2api',
          provider: body.provider,
          base_url: normalizeBaseURL(body.base_url),
          proxy_id: body.proxy_id && body.proxy_id > 0 ? body.proxy_id : undefined,
          weight: body.weight ?? 10,
          accounts: slice,
        });
        totalImported += r.imported;
        totalSkipped += r.skipped;
      }
      const sk = totalSkipped > 0 ? `，跳过 ${totalSkipped}` : '';
      toast.success(`完成：${totalImported} 条${sk}`);
      onSuccess();
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '导入失败');
    } finally {
      setSub2Busy(false);
    }
  };

  return (
    <Modal title="批量导入" onClose={onClose} wide>
      <div className="space-y-2">
        <div className="flex flex-wrap gap-1.5">
          <button
            type="button"
            className={`btn btn-sm ${importMode === 'lines' ? 'btn-primary' : 'btn-outline'}`}
            onClick={() => setImportMode('lines')}
          >
            文本
          </button>
          <button
            type="button"
            className={`btn btn-sm ${importMode === 'sub2api' ? 'btn-primary' : 'btn-outline'}`}
            onClick={() => setImportMode('sub2api')}
          >
            JSON
          </button>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <Field label="Provider">
            <select
              className="select select-sm"
              value={body.provider}
              onChange={(e) => {
                const p = e.target.value as 'gpt' | 'grok';
                setBody((s) => ({ ...s, provider: p, auth_type: defaultAuthType(p) }));
              }}
            >
              <option value="gpt">GPT</option>
              <option value="grok">GROK</option>
            </select>
          </Field>
          {importMode === 'lines' ? (
            <Field label="类型">
              <select
                className="select select-sm"
                value={body.auth_type}
                onChange={(e) =>
                  setBody((s) => ({
                    ...s,
                    auth_type: e.target.value as 'api_key' | 'oauth' | 'cookie',
                  }))
                }
              >
                <option value="api_key">API Key</option>
                <option value="oauth">OAuth</option>
                <option value="cookie">Grok Token</option>
              </select>
            </Field>
          ) : (
            <Field label="每批">
              <input
                type="number"
                className="input input-sm"
                min={50}
                max={500}
                value={sub2ChunkSize}
                onChange={(e) =>
                  setSub2ChunkSize(Math.min(500, Math.max(50, Number(e.target.value) || 300)))
                }
              />
            </Field>
          )}
          <Field label="权重">
            <input
              type="number"
              className="input input-sm"
              min={1}
              max={1000}
              value={body.weight ?? 10}
              onChange={(e) => setBody((s) => ({ ...s, weight: Number(e.target.value) || 10 }))}
            />
          </Field>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <Field label="Base URL">
            <input
              className="input input-sm"
              placeholder="可选"
              value={body.base_url || ''}
              onChange={(e) => setBody((s) => ({ ...s, base_url: e.target.value }))}
            />
          </Field>
          <Field label="代理">
            <select
              className="select select-sm"
              value={body.proxy_id ?? 0}
              onChange={(e) => {
                const n = Number(e.target.value) || 0;
                setBody((s) => ({ ...s, proxy_id: n > 0 ? n : undefined }));
              }}
            >
              <option value={0}>无</option>
              {proxies.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </Field>
        </div>

        {importMode === 'lines' ? (
          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (!body.text?.trim()) {
                toast.error('请粘贴账号列表');
                return;
              }
              if (!body.auth_type) {
                toast.error('请选择凭证类型');
                return;
              }
              m.mutate({
                format: 'lines',
                provider: body.provider,
                auth_type: body.auth_type,
                base_url: normalizeBaseURL(body.base_url),
                proxy_id: body.proxy_id && body.proxy_id > 0 ? body.proxy_id : undefined,
                weight: body.weight ?? 10,
                text: body.text,
              });
            }}
          >
            <Field label="每行一条">
              <textarea
                className="textarea textarea-sm font-mono text-small min-h-[140px]"
                placeholder={linePlaceholder}
                value={body.text ?? ''}
                onChange={(e) => setBody((s) => ({ ...s, text: e.target.value }))}
              />
            </Field>
            <div className="flex justify-end gap-2 pt-1">
              <button type="button" className="btn btn-outline btn-sm" onClick={onClose}>
                取消
              </button>
              <button type="submit" className="btn btn-primary btn-sm" disabled={m.isPending}>
                {m.isPending ? '…' : '导入'}
              </button>
            </div>
          </form>
        ) : (
          <div className="space-y-2">
            <Field label="文件">
              <div className="flex flex-wrap items-center gap-2">
                <label className="btn btn-outline btn-sm cursor-pointer">
                  选择
                  <input
                    type="file"
                    accept=".json,application/json"
                    className="hidden"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (!f) return;
                      const reader = new FileReader();
                      reader.onload = () => {
                        try {
                          const list = parseSub2ExportJson(String(reader.result ?? ''));
                          setSub2Accounts(list);
                          setSub2FileLabel(`${f.name} · ${list.length}`);
                          toast.success(`已解析 ${list.length} 条`);
                        } catch (err) {
                          setSub2Accounts(null);
                          setSub2FileLabel('');
                          toast.error(err instanceof Error ? err.message : '解析失败');
                        }
                      };
                      reader.readAsText(f, 'UTF-8');
                      e.target.value = '';
                    }}
                  />
                </label>
                <span className="text-tiny text-text-secondary truncate max-w-[240px]">
                  {sub2FileLabel || '未选'}
                </span>
              </div>
            </Field>
            <div className="flex justify-end gap-2 pt-1">
              <button type="button" className="btn btn-outline btn-sm" onClick={onClose}>
                取消
              </button>
              <button
                type="button"
                className="btn btn-primary btn-sm"
                disabled={sub2Busy || !sub2Accounts?.length}
                onClick={() => void runSub2Import()}
              >
                {sub2Busy ? '…' : '导入'}
              </button>
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}

// ============== Edit Dialog ==============
function EditDialog({
  item,
  onClose,
  onSuccess,
}: {
  item: AccountItem;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const isOAuth = item.auth_type === 'oauth';
  const [body, setBody] = useState<AccountUpdateBody>({
    name: item.name,
    weight: item.weight,
    rpm_limit: item.rpm_limit,
    tpm_limit: item.tpm_limit,
    daily_quota: item.daily_quota,
    monthly_quota: item.monthly_quota,
    remark: item.remark ?? '',
    access_token: '',
    refresh_token: '',
    session_token: '',
    client_id: '',
    credential: '',
  });
  const [showAdvanced, setShowAdvanced] = useState(false);

  // 拉取明文凭证回填（管理员专用）
  const secretsQ = useQuery({
    queryKey: ['admin', 'accounts', item.id, 'secrets'],
    queryFn: () => accountsApi.secrets(item.id),
    staleTime: 0,
    gcTime: 0,
  });
  // 记录初始明文，提交时只发送被改动的字段，避免无意义重复加密 / 清空 AT。
  const initialRef = useRef({
    access_token: '',
    refresh_token: '',
    session_token: '',
    client_id: '',
    credential: '',
  });
  useEffect(() => {
    const s = secretsQ.data;
    if (!s) return;
    const next = {
      access_token: s.access_token ?? '',
      refresh_token: s.refresh_token ?? '',
      session_token: s.session_token ?? '',
      client_id: s.client_id ?? '',
      credential: s.credential ?? '',
    };
    initialRef.current = next;
    setBody((prev) => ({ ...prev, ...next }));
  }, [secretsQ.data]);

  const m = useMutation({
    mutationFn: (b: AccountUpdateBody) => accountsApi.update(item.id, b),
    onSuccess: () => {
      toast.success('已保存');
      onSuccess();
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  return (
    <Modal title={`编辑账号 · ${item.name}`} onClose={onClose}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          const name = (body.name ?? '').trim();
          if (!name) {
            toast.error('请填写名称');
            return;
          }
          const payload: AccountUpdateBody = {
            name,
            weight: body.weight,
            rpm_limit: Math.max(0, body.rpm_limit ?? 0),
            tpm_limit: Math.max(0, body.tpm_limit ?? 0),
            daily_quota: Math.max(0, body.daily_quota ?? 0),
            monthly_quota: Math.max(0, body.monthly_quota ?? 0),
            remark: (body.remark ?? '').trim(),
          };
          const init = initialRef.current;
          if (isOAuth) {
            const at = (body.access_token ?? '').trim();
            const rt = (body.refresh_token ?? '').trim();
            const st = (body.session_token ?? '').trim();
            const cid = (body.client_id ?? '').trim();
            if (at !== init.access_token) payload.access_token = at;
            if (rt !== init.refresh_token) payload.refresh_token = rt;
            if (st !== init.session_token) payload.session_token = st;
            if (cid !== init.client_id) payload.client_id = cid;
          } else {
            const c = (body.credential ?? '').trim();
            if (c !== init.credential) payload.credential = c;
          }
          m.mutate(payload);
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Provider" hint="创建后不可更改">
            <input
              className="input bg-bg-elevated text-text-secondary"
              value={item.provider.toUpperCase()}
              readOnly
              disabled
            />
          </Field>
          <Field label="名称 / 标签">
            <input
              className="input"
              value={body.name ?? ''}
              onChange={(e) => setBody((s) => ({ ...s, name: e.target.value }))}
            />
          </Field>
        </div>

        {isOAuth ? (
          <div className="space-y-3">
            <div className="text-tiny text-text-tertiary inline-flex items-center gap-2">
              {secretsQ.isLoading ? (
                <>
                  <RotateCw size={12} className="animate-spin" />
                  正在解密读取已有凭证…
                </>
              ) : secretsQ.isError ? (
                <span className="text-warning inline-flex items-center gap-1">
                  <AlertCircle size={12} /> 读取明文失败，留空表示保持原值
                </span>
              ) : (
                <span>已读取原始凭证，可直接修改后保存（清空表示移除该字段）</span>
              )}
              <span className="ml-1 badge badge-outline text-tiny">RT {item.has_refresh_token ? '✓' : '✗'}</span>
              <span className="badge badge-outline text-tiny">AT {item.has_access_token ? '✓' : '∅'}</span>
            </div>
            <Field label="Access Token" hint="修改后会重新解析新的过期时间；清空则移除">
              <textarea
                className="textarea font-mono text-small min-h-[64px]"
                placeholder={secretsQ.isLoading ? '加载中…' : '可清空'}
                value={body.access_token ?? ''}
                onChange={(e) => setBody((s) => ({ ...s, access_token: e.target.value }))}
              />
            </Field>
            <Field label="Refresh Token" hint="修改后会替换并清空 Access Token；清空则移除">
              <textarea
                className="textarea font-mono text-small min-h-[64px]"
                placeholder={secretsQ.isLoading ? '加载中…' : '可清空'}
                value={body.refresh_token ?? ''}
                onChange={(e) => setBody((s) => ({ ...s, refresh_token: e.target.value }))}
              />
            </Field>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Field label="Session Token" hint="清空则移除">
                <input
                  className="input font-mono text-small"
                  placeholder={secretsQ.isLoading ? '加载中…' : '可清空'}
                  value={body.session_token ?? ''}
                  onChange={(e) => setBody((s) => ({ ...s, session_token: e.target.value }))}
                />
              </Field>
              <Field label="Client ID" hint="清空则使用系统默认">
                <input
                  className="input font-mono text-small"
                  placeholder={secretsQ.isLoading ? '加载中…' : '可清空使用系统默认'}
                  value={body.client_id ?? ''}
                  onChange={(e) => setBody((s) => ({ ...s, client_id: e.target.value }))}
                />
              </Field>
            </div>
          </div>
        ) : (
          <Field
            label="API Key"
            hint={
              secretsQ.isLoading
                ? '正在解密读取…'
                : secretsQ.isError
                  ? '读取明文失败，留空表示保持原值'
                  : '已读取原始凭证；保存前 AES-256-GCM 加密落库'
            }
          >
            <textarea
              className="textarea font-mono text-small min-h-[80px]"
              placeholder={secretsQ.isLoading ? '加载中…' : 'xai-xxxxxxxxxxxxxxxxxxxxxxxx'}
              value={body.credential ?? ''}
              onChange={(e) => setBody((s) => ({ ...s, credential: e.target.value }))}
            />
          </Field>
        )}

        <div className="grid grid-cols-2 gap-3">
          <Field label="权重" hint="加权轮询时的相对权重，范围 1–1000">
            <input
              type="number"
              className="input"
              min={1}
              max={1000}
              value={body.weight ?? 10}
              onChange={(e) => setBody((s) => ({ ...s, weight: Number(e.target.value) || 10 }))}
            />
          </Field>
          <Field label="备注（可选）">
            <input
              className="input"
              value={body.remark ?? ''}
              onChange={(e) => setBody((s) => ({ ...s, remark: e.target.value }))}
            />
          </Field>
        </div>

        <button
          type="button"
          className="btn btn-ghost btn-sm gap-1"
          onClick={() => setShowAdvanced((v) => !v)}
        >
          {showAdvanced ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          高级（限速 / 配额）
        </button>
        {showAdvanced && (
          <div className="card card-flat p-3 space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <Field label="RPM 限速" hint="每分钟最大请求数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.rpm_limit ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({ ...s, rpm_limit: Math.max(0, Number(e.target.value) || 0) }))
                  }
                />
              </Field>
              <Field label="TPM 限速" hint="每分钟最大 token 数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.tpm_limit ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({ ...s, tpm_limit: Math.max(0, Number(e.target.value) || 0) }))
                  }
                />
              </Field>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="每日配额" hint="单日最大调用次数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.daily_quota ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({ ...s, daily_quota: Math.max(0, Number(e.target.value) || 0) }))
                  }
                />
              </Field>
              <Field label="每月配额" hint="单月最大调用次数；0 不限">
                <input
                  type="number"
                  className="input"
                  min={0}
                  value={body.monthly_quota ?? 0}
                  onChange={(e) =>
                    setBody((s) => ({
                      ...s,
                      monthly_quota: Math.max(0, Number(e.target.value) || 0),
                    }))
                  }
                />
              </Field>
            </div>
          </div>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn btn-outline btn-md" onClick={onClose}>
            取消
          </button>
          <button type="submit" className="btn btn-primary btn-md" disabled={m.isPending}>
            {m.isPending ? '保存中…' : '保存'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

// ============== Purge-All Confirm Dialog ==============
function PurgeAllDialog({
  provider,
  loading,
  onClose,
  onConfirm,
}: {
  provider?: 'gpt' | 'grok';
  loading: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const KEYWORD = '删除全部';
  const [text, setText] = useState('');
  const ok = text.trim() === KEYWORD;
  const scopeLabel = provider ? `${provider.toUpperCase()} Provider` : '全部 Provider';

  return (
    <Modal title="软删全部账号 · 危险操作" onClose={onClose}>
      <div className="space-y-4">
        <div className="card card-flat p-3 border border-danger/40 bg-danger/5">
          <div className="flex items-start gap-2">
            <AlertCircle size={18} className="text-danger mt-0.5 shrink-0" />
            <div className="space-y-1 text-small">
              <p className="text-text-primary font-medium">即将软删 {scopeLabel} 下的全部账号</p>
              <p className="text-text-secondary">
                忽略当前搜索过滤；操作不可在前端撤销。被软删的账号会从池中下线，但记录会保留以便事后追查。
              </p>
            </div>
          </div>
        </div>

        <label className="field">
          <span className="field-label">
            请输入 <span className="text-danger font-semibold mx-0.5">{KEYWORD}</span> 以继续
          </span>
          <input
            autoFocus
            className={`input ${text && !ok ? 'border-danger focus:border-danger' : ''}`}
            placeholder={KEYWORD}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && ok && !loading) {
                e.preventDefault();
                onConfirm();
              }
            }}
          />
          <span className="field-hint">逐字输入，区分大小写</span>
        </label>

        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn btn-outline btn-md" onClick={onClose} disabled={loading}>
            取消
          </button>
          <button
            type="button"
            className="btn btn-danger btn-md gap-1"
            disabled={!ok || loading}
            onClick={onConfirm}
          >
            <Trash2 size={14} />
            {loading ? '删除中…' : '确认删除全部'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

// ============== UI helpers ==============
function Modal({
  title,
  onClose,
  wide,
  children,
}: {
  title: string;
  onClose: () => void;
  /** 宽屏弹层（批量导入 sub2api 表单） */
  wide?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-[80] grid place-items-center bg-black/40 backdrop-blur-sm p-4">
      <div className={`dialog-surface w-full ${wide ? 'max-w-2xl' : 'max-w-xl'} klein-fade-in`}>
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
