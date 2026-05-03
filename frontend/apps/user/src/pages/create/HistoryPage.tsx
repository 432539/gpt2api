import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import clsx from 'clsx';
import { ImageIcon, Video as VideoIcon, Loader2, Images } from 'lucide-react';

import { fmtPoints, fmtRelative } from '../../lib/format';
import { genApi } from '../../lib/services';
import type { GenerationTask, TaskStatus } from '../../lib/types';

const STATUS_LABEL: Record<TaskStatus, string> = {
  0: '排队中', 1: '生成中', 2: '已完成', 3: '失败', 4: '已退款', 5: '已取消',
};
const STATUS_BADGE: Record<TaskStatus, string> = {
  0: 'badge', 1: 'badge badge-klein', 2: 'badge badge-success',
  3: 'badge badge-danger', 4: 'badge badge-warning', 5: 'badge',
};

type Filter = 'all' | 'image' | 'video';
const FILTERS: Array<{ value: Filter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'image', label: '图像' },
  { value: 'video', label: '视频' },
];

export default function HistoryPage() {
  const [filter, setFilter] = useState<Filter>('all');
  const [page, setPage] = useState(1);

  const q = useQuery({
    queryKey: ['gen.history', filter, page],
    queryFn: () =>
      genApi.history({ kind: filter === 'all' ? undefined : filter, page, page_size: 24 }),
  });

  const items = q.data?.list ?? [];
  const total = q.data?.total ?? 0;

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1 className="page-title">生成历史</h1>
          <p className="page-subtitle">最近的图像 / 视频作品（保留 14 天）</p>
        </div>
        <div className="tabs">
          {FILTERS.map((f) => (
            <button
              key={f.value}
              type="button"
              className="tab"
              aria-selected={filter === f.value}
              onClick={() => { setFilter(f.value); setPage(1); }}
            >
              {f.label}
            </button>
          ))}
        </div>
      </header>

      {q.isLoading && (
        <div className="grid place-items-center py-20 text-text-tertiary">
          <Loader2 className="animate-spin" size={28} />
        </div>
      )}

      {!q.isLoading && items.length === 0 && (
        <div className="card">
          <div className="empty-state">
            <span className="empty-state-icon">
              <Images size={22} />
            </span>
            <p className="empty-state-title">还没有任何作品</p>
            <p className="empty-state-desc">去「图像创作」或「视频创作」开始你的第一次生成吧。</p>
          </div>
        </div>
      )}

      <div className="grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(220px,100%),1fr))]">
        {items.map((t) => (
          <TaskCard key={t.task_id} t={t} />
        ))}
      </div>

      {total > items.length && (
        <div className="mt-6 flex justify-center">
          <button
            className="btn btn-outline btn-md"
            onClick={() => setPage((p) => p + 1)}
            disabled={q.isFetching}
          >
            加载更多
          </button>
        </div>
      )}
    </div>
  );
}

function TaskCard({ t }: { t: GenerationTask }) {
  const cover = t.results?.[0]?.thumb_url || t.results?.[0]?.url;
  const isVideo = t.kind === 'video';
  return (
    <article className="group rounded-lg overflow-hidden bg-surface-1 border border-border hover:shadow-glow-soft hover:-translate-y-0.5 transition">
      <div
        className="relative aspect-square bg-klein-gradient-soft grid place-items-center overflow-hidden"
        style={{ contain: 'paint' }}
      >
        {cover ? (
          isVideo ? (
            <video src={cover} className="h-full w-full object-cover" muted playsInline />
          ) : (
            <img src={cover} alt="" className="h-full w-full object-cover" loading="lazy" />
          )
        ) : (
          <div className="text-text-tertiary flex flex-col items-center gap-1">
            {isVideo ? <VideoIcon size={28} /> : <ImageIcon size={28} />}
            <span className="text-small">暂无预览</span>
          </div>
        )}
        {t.status === 1 && (
          <div className="absolute inset-x-0 bottom-0 progress" style={{ borderRadius: 0 }}>
            <div className="progress-bar" style={{ width: `${t.progress}%`, borderRadius: 0 }} />
          </div>
        )}
      </div>
      <div className="p-3 space-y-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="font-medium text-small text-text-primary truncate">{t.model}</span>
          <span className={clsx(STATUS_BADGE[t.status])}>{STATUS_LABEL[t.status]}</span>
        </div>
        <div className="flex items-center justify-between text-tiny text-text-tertiary">
          <span>{fmtRelative(t.created_at)}</span>
          <span>{fmtPoints(t.cost_points)} 点</span>
        </div>
      </div>
    </article>
  );
}
