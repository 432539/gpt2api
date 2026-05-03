import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import clsx from 'clsx';
import {
  Copy,
  Download,
  ImageIcon,
  Images,
  Loader2,
  MoreHorizontal,
  Play,
  Trash2,
  Video as VideoIcon,
  X,
} from 'lucide-react';

import { fmtPoints, fmtRelative } from '../../lib/format';
import { loadToken } from '../../lib/api';
import { genApi } from '../../lib/services';
import type { GenerationTask, TaskStatus } from '../../lib/types';

const PAGE_SIZE = 20;

const STATUS_LABEL: Record<TaskStatus, string> = {
  0: '排队中',
  1: '生成中',
  2: '已完成',
  3: '失败',
  4: '已退款',
  5: '已取消',
};

const STATUS_BADGE: Record<TaskStatus, string> = {
  0: 'badge',
  1: 'badge badge-klein',
  2: 'badge badge-success',
  3: 'badge badge-danger',
  4: 'badge badge-warning',
  5: 'badge',
};

type Filter = 'all' | 'image' | 'video';
type DeleteScope = 'failed' | 'before_3d' | 'before_7d' | 'all';

const FILTERS: Array<{ value: Filter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'image', label: '图片' },
  { value: 'video', label: '视频' },
];

const DELETE_ACTIONS: Array<{ scope: DeleteScope; label: string; hint: string }> = [
  { scope: 'failed', label: '删除失败', hint: '清理生成失败的记录' },
  { scope: 'before_3d', label: '删除3天前', hint: '删除3天前的历史记录' },
  { scope: 'before_7d', label: '删除7天前', hint: '删除7天前的历史记录' },
  { scope: 'all', label: '删除全部', hint: '清空全部历史记录' },
];

export default function HistoryPage() {
  const [filter, setFilter] = useState<Filter>('all');
  const [page, setPage] = useState(1);
  const [menuOpen, setMenuOpen] = useState(false);
  const [busyScope, setBusyScope] = useState<DeleteScope | null>(null);
  const [confirmScope, setConfirmScope] = useState<DeleteScope | null>(null);
  const [preview, setPreview] = useState<HistoryPreview | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);

  const q = useQuery({
    queryKey: ['gen.history', filter, page],
    queryFn: () =>
      genApi.history({ kind: filter === 'all' ? undefined : filter, page, page_size: PAGE_SIZE }),
  });

  const items = q.data?.list ?? [];
  const total = q.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  useEffect(() => {
    const onDocClick = (ev: MouseEvent) => {
      if (!menuRef.current) return;
      if (!menuRef.current.contains(ev.target as Node)) setMenuOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, []);

  const handleDelete = async (scope: DeleteScope) => {
    setBusyScope(scope);
    try {
      await genApi.deleteHistory(scope);
      setPage(1);
      await q.refetch();
    } finally {
      setBusyScope(null);
      setMenuOpen(false);
    }
  };

  return (
    <div className="page">
      <header className="page-header items-start gap-4">
        <div className="space-y-2">
          <h1 className="page-title">生成历史</h1>
          <p className="page-subtitle">图片和视频都可以预览、下载、复制链接，也可以按时间和失败状态清理。</p>
        </div>

        <div className="flex items-center gap-3">
          <div className="tabs">
            {FILTERS.map((f) => (
              <button
                key={f.value}
                type="button"
                className="tab"
                aria-selected={filter === f.value}
                onClick={() => {
                  setFilter(f.value);
                  setPage(1);
                }}
              >
                {f.label}
              </button>
            ))}
          </div>

          <div className="relative" ref={menuRef}>
            <button
              type="button"
              className="btn btn-outline btn-md gap-2"
              onClick={() => setMenuOpen((v) => !v)}
            >
              <MoreHorizontal size={16} />
              管理
            </button>
            {menuOpen && (
              <div className="absolute right-0 top-[calc(100%+8px)] z-30 w-48 rounded-xl border border-border bg-surface-1 p-2 shadow-lg">
                {DELETE_ACTIONS.map((item) => (
                  <button
                    key={item.scope}
                    type="button"
                    className={clsx(
                      'flex w-full items-start gap-3 rounded-lg px-3 py-2 text-left text-sm transition hover:bg-surface-2',
                      busyScope === item.scope && 'opacity-60 pointer-events-none',
                    )}
                    onClick={() => setConfirmScope(item.scope)}
                  >
                    <Trash2 size={16} className="mt-0.5 text-text-tertiary" />
                    <span className="flex-1">
                      <span className="block text-text-primary">{item.label}</span>
                      <span className="block text-xs text-text-tertiary">{item.hint}</span>
                    </span>
                    {busyScope === item.scope && <Loader2 size={14} className="animate-spin" />}
                  </button>
                ))}
              </div>
            )}
          </div>

          <button
            type="button"
            className="btn btn-outline btn-md"
            onClick={() => q.refetch()}
            disabled={q.isFetching}
          >
            <Loader2 size={16} className={clsx(q.isFetching && 'animate-spin')} />
            刷新
          </button>
        </div>
      </header>

      <section className="mb-4 flex items-center justify-between text-sm text-text-tertiary">
        <span>共 {total} 条记录</span>
        <span>
          第 {page} / {totalPages} 页
        </span>
      </section>

      {q.isLoading && (
        <div className="grid place-items-center py-20 text-text-tertiary">
          <Loader2 className="animate-spin" size={28} />
        </div>
      )}

      {!q.isLoading && q.error && (
        <div className="card">
          <div className="empty-state">
            <span className="empty-state-icon">
              <Trash2 size={22} />
            </span>
            <p className="empty-state-title">加载失败</p>
            <p className="empty-state-desc">请稍后刷新重试，或者检查登录状态。</p>
          </div>
        </div>
      )}

      {!q.isLoading && !q.error && items.length === 0 && (
        <div className="card">
          <div className="empty-state">
            <span className="empty-state-icon">
              <Images size={22} />
            </span>
            <p className="empty-state-title">还没有任何作品</p>
            <p className="empty-state-desc">去图片创作或视频创作开始你的第一次生成吧。</p>
          </div>
        </div>
      )}

      {!q.isLoading && items.length > 0 && (
        <>
          <div className="grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(240px,100%),1fr))]">
            {items.map((t) => (
              <TaskCard key={t.task_id} t={t} onPreview={() => setPreview(createPreview(t))} />
            ))}
          </div>

          <div className="mt-6 flex items-center justify-center gap-3">
            <button
              className="btn btn-outline btn-md"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1 || q.isFetching}
            >
              上一页
            </button>
            <button
              className="btn btn-outline btn-md"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages || q.isFetching}
            >
              下一页
            </button>
          </div>
        </>
      )}

      {preview && <PreviewModal preview={preview} onClose={() => setPreview(null)} />}
      {confirmScope && (
        <DeleteConfirmDialog
          scope={confirmScope}
          loading={busyScope === confirmScope}
          onClose={() => setConfirmScope(null)}
          onConfirm={async () => {
            const scope = confirmScope;
            if (!scope) return;
            setConfirmScope(null);
            await handleDelete(scope);
          }}
        />
      )}
    </div>
  );
}

function TaskCard({ t, onPreview }: { t: GenerationTask; onPreview: () => void }) {
  const primary = t.results?.[0];
  const cover = primary?.thumb_url || primary?.url || '';
  const isVideo = t.kind === 'video';
  const resolvedCover = useAuthedMediaUrl(cover);
  const error = t.status === 3 ? t.error?.trim() || '生成失败' : '';

  return (
    <article
      className="group overflow-hidden rounded-lg border border-border bg-surface-1 transition hover:-translate-y-0.5 hover:shadow-glow-soft"
      role="button"
      tabIndex={0}
      onClick={onPreview}
      onKeyDown={(ev) => {
        if (ev.key === 'Enter' || ev.key === ' ') onPreview();
      }}
    >
      <div className="relative aspect-square overflow-hidden bg-klein-gradient-soft" style={{ contain: 'paint' }}>
        {resolvedCover ? (
          isVideo ? (
            <div className="relative h-full w-full">
              <img src={resolvedCover} alt="" className="h-full w-full object-cover" loading="lazy" />
              <div className="absolute inset-0 grid place-items-center bg-black/10 opacity-100 transition group-hover:bg-black/20">
                <span className="flex h-12 w-12 items-center justify-center rounded-full bg-black/70 text-white">
                  <Play size={18} className="ml-0.5" fill="currentColor" />
                </span>
              </div>
            </div>
          ) : (
            <img src={resolvedCover} alt="" className="h-full w-full object-cover" loading="lazy" />
          )
        ) : (
          <div className="flex h-full w-full items-center justify-center text-text-tertiary">
            {isVideo ? <VideoIcon size={28} /> : <ImageIcon size={28} />}
          </div>
        )}
        {t.status === 1 && (
          <div className="absolute inset-x-0 bottom-0 progress">
            <div className="progress-bar" style={{ width: `${t.progress}%` }} />
          </div>
        )}
        {t.status === 3 && (
          <div className="absolute inset-0 flex items-end bg-black/15 p-3">
            <span className="line-clamp-2 rounded-md bg-black/60 px-2 py-1 text-xs text-white">
              {error}
            </span>
          </div>
        )}
      </div>

      <div className="space-y-2 p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="truncate text-sm text-text-primary">{t.model}</span>
          <span className={clsx(STATUS_BADGE[t.status])}>{STATUS_LABEL[t.status]}</span>
        </div>
        <div className="flex items-center justify-between text-xs text-text-tertiary">
          <span>{fmtRelative(t.created_at)}</span>
          <span>{fmtPoints(t.cost_points)} 点</span>
        </div>
      </div>
    </article>
  );
}

function PreviewModal({ preview, onClose }: { preview: HistoryPreview; onClose: () => void }) {
  const blobUrl = useAuthedMediaUrl(preview.src);
  const [copying, setCopying] = useState(false);
  const [downloading, setDownloading] = useState(false);

  useEffect(() => {
    const onKeyDown = (ev: KeyboardEvent) => {
      if (ev.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  const handleCopy = async () => {
    setCopying(true);
    try {
      await navigator.clipboard.writeText(preview.src);
    } finally {
      setCopying(false);
    }
  };

  const handleDownload = async () => {
    setDownloading(true);
    try {
      const file = await fetchAuthedFile(preview.src);
      const url = URL.createObjectURL(file.blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = file.filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <div
        className="w-full max-w-5xl overflow-hidden rounded-2xl border border-border bg-surface-1 shadow-2xl"
        onClick={(ev) => ev.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
          <div className="min-w-0">
            <p className="truncate text-sm text-text-primary">{preview.model}</p>
            <p className="text-xs text-text-tertiary">
              {fmtRelative(preview.created_at)} · {STATUS_LABEL[preview.status]} · {fmtPoints(preview.cost_points)} 点
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button className="btn btn-outline btn-sm gap-2" onClick={handleCopy} disabled={copying}>
              {copying ? <Loader2 size={14} className="animate-spin" /> : <Copy size={14} />}
              复制链接
            </button>
            <button className="btn btn-outline btn-sm gap-2" onClick={handleDownload} disabled={downloading}>
              {downloading ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
              下载
            </button>
            <button className="btn btn-outline btn-sm" onClick={onClose}>
              <X size={14} />
            </button>
          </div>
        </div>

        <div className="bg-black/5 p-4">
          <div className="flex max-h-[75vh] min-h-[360px] items-center justify-center overflow-auto rounded-xl bg-surface-0">
            {preview.kind === 'video' ? (
              blobUrl ? (
                <video src={blobUrl} controls className="max-h-[75vh] w-full object-contain" />
              ) : (
                <div className="flex flex-col items-center gap-2 py-20 text-text-tertiary">
                  <Loader2 className="animate-spin" size={24} />
                  <span className="text-sm">正在加载视频</span>
                </div>
              )
            ) : blobUrl ? (
              <img src={blobUrl} alt={preview.prompt || preview.model} className="max-h-[75vh] w-full object-contain" />
            ) : (
              <div className="flex flex-col items-center gap-2 py-20 text-text-tertiary">
                <Loader2 className="animate-spin" size={24} />
                <span className="text-sm">正在加载图片</span>
              </div>
            )}
          </div>
        </div>

        <div className="border-t border-border px-4 py-3 text-sm text-text-tertiary">
          <p className="line-clamp-2">{preview.prompt || '无提示词'}</p>
        </div>
      </div>
    </div>
  );
}

function useAuthedMediaUrl(src?: string) {
  const [url, setUrl] = useState<string>('');

  useEffect(() => {
    if (!src) {
      setUrl('');
      return;
    }
    if (src.startsWith('data:')) {
      setUrl(src);
      return;
    }

    let alive = true;
    let objectUrl = '';

    (async () => {
      try {
        const token = loadToken();
        const headers: Record<string, string> = {};
        if (token?.access) headers.Authorization = `${token.type || 'Bearer'} ${token.access}`;
        const resp = await fetch(src, { headers, credentials: 'include' });
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const blob = await resp.blob();
        if (!alive) return;
        objectUrl = URL.createObjectURL(blob);
        setUrl(objectUrl);
      } catch {
        if (alive) setUrl('');
      }
    })();

    return () => {
      alive = false;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [src]);

  return url;
}

async function fetchAuthedFile(src: string) {
  const token = loadToken();
  const headers: Record<string, string> = {};
  if (token?.access) headers.Authorization = `${token.type || 'Bearer'} ${token.access}`;
  const resp = await fetch(src, { headers, credentials: 'include' });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  const blob = await resp.blob();
  return {
    blob,
    filename: guessFilename(src, resp.headers.get('content-type') || blob.type),
  };
}

function guessFilename(src: string, contentType: string) {
  const ext = guessExt(contentType, src);
  const cleanSrc = src.replace(/\?.*$/, '');
  const base = cleanSrc.replace(/^.*\//, '').replace(/[^a-zA-Z0-9._-]+/g, '_').slice(0, 80);
  return `${base || 'generation'}${ext}`;
}

function guessExt(contentType: string, src: string) {
  const lower = `${contentType} ${src}`.toLowerCase();
  if (lower.includes('video/mp4') || lower.includes('.mp4')) return '.mp4';
  if (lower.includes('video/webm') || lower.includes('.webm')) return '.webm';
  if (lower.includes('image/png') || lower.includes('.png')) return '.png';
  if (lower.includes('image/jpeg') || lower.includes('image/jpg') || lower.includes('.jpg') || lower.includes('.jpeg')) return '.jpg';
  if (lower.includes('image/webp') || lower.includes('.webp')) return '.webp';
  return '';
}

function createPreview(t: GenerationTask): HistoryPreview {
  const first = t.results?.[0];
  return {
    kind: t.kind,
    status: t.status,
    model: t.model,
    prompt: t.prompt || '',
    cost_points: t.cost_points,
    created_at: t.created_at,
    error: t.error,
    src: first?.url || first?.thumb_url || '',
  };
}

interface HistoryPreview {
  kind: 'image' | 'video' | 'chat';
  status: TaskStatus;
  model: string;
  prompt: string;
  cost_points: number;
  created_at: number;
  error?: string;
  src: string;
}

function DeleteConfirmDialog({
  scope,
  loading,
  onClose,
  onConfirm,
}: {
  scope: DeleteScope;
  loading: boolean;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
}) {
  const item = DELETE_ACTIONS.find((a) => a.scope === scope)!;
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/55 p-4" onClick={onClose}>
      <div
        className="dialog-surface w-full max-w-md overflow-hidden rounded-2xl border border-border bg-surface-1 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3 border-b border-border px-5 py-4">
          <div className="grid h-10 w-10 place-items-center rounded-full bg-danger-soft text-danger">
            <Trash2 size={18} />
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="text-h4 text-text-primary">{item.label}</h2>
            <p className="mt-1 text-small text-text-tertiary">确定执行这个操作吗？该操作不可恢复。</p>
          </div>
          <button type="button" className="btn btn-ghost btn-icon btn-sm" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <div className="px-5 py-4">
          <p className="text-small text-text-secondary">{item.hint}</p>
          <div className="mt-5 flex justify-end gap-2">
            <button type="button" className="btn btn-outline btn-md" onClick={onClose}>
              取消
            </button>
            <button type="button" className="btn btn-danger btn-md gap-2" onClick={onConfirm} disabled={loading}>
              {loading ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
              确认删除
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
