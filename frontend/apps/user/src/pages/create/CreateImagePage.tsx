import { useEffect, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Sparkles, Upload, RefreshCw, Heart, Download, Loader2 } from 'lucide-react';
import clsx from 'clsx';

import { useEnsureLoggedIn } from '../../hooks/useEnsureLoggedIn';
import { ApiError } from '../../lib/api';
import { fmtPoints } from '../../lib/format';
import { genApi } from '../../lib/services';
import type { GenerationTask } from '../../lib/types';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

const MODELS = [
  { code: 'img-v3',    name: '通用 V3.0',    hot: true,  cost: 4 },
  { code: 'img-real',  name: '写实 V2.1',    cost: 4 },
  { code: 'img-anime', name: '二次元 V2.0',  cost: 3 },
  { code: 'img-3d',    name: '3D 渲染',       cost: 5,    pro: true },
];

const RATIOS = ['1:1', '3:4', '4:3', '16:9', '9:16'] as const;
const COUNTS = [1, 2, 4] as const;
const QUALITY = [
  { value: 'standard', label: '标准' },
  { value: 'hd', label: '高清' },
] as const;

type Quality = (typeof QUALITY)[number]['value'];

const STATUS_LABELS: Record<number, string> = {
  0: '排队中', 1: '生成中', 2: '已完成', 3: '失败', 4: '已退款', 5: '已取消',
};

export default function CreateImagePage() {
  const qc = useQueryClient();
  const refreshMe = useAuthStore((s) => s.refreshMe);
  const ensureLoggedIn = useEnsureLoggedIn();

  const [model, setModel] = useState(MODELS[0]!.code);
  const [ratio, setRatio] = useState<(typeof RATIOS)[number]>('1:1');
  const [count, setCount] = useState<(typeof COUNTS)[number]>(2);
  const [quality, setQuality] = useState<Quality>('hd');
  const [prompt, setPrompt] = useState('一座漂浮在云端的未来主义城堡，黄昏，体积光，电影级构图');

  const [task, setTask] = useState<GenerationTask | null>(null);
  const pollRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current);
    };
  }, []);

  const createMut = useMutation({
    mutationFn: () =>
      genApi.createImage({ model, prompt, count, ratio, quality }),
    onSuccess: (t) => {
      setTask(t);
      startPolling(t.task_id);
      void refreshMe();
      qc.invalidateQueries({ queryKey: ['gen.history'] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '生成失败'),
  });

  const startPolling = (taskId: string) => {
    if (pollRef.current) window.clearInterval(pollRef.current);
    pollRef.current = window.setInterval(async () => {
      try {
        const fresh = await genApi.getTask(taskId);
        setTask(fresh);
        if (fresh.status === 2 || fresh.status === 3 || fresh.status === 4) {
          if (pollRef.current) window.clearInterval(pollRef.current);
          pollRef.current = null;
          if (fresh.status === 2) toast.success('生成完成');
          else if (fresh.status === 3) toast.error(fresh.error || '生成失败');
          else toast.info('生成失败已退款');
          await refreshMe();
          qc.invalidateQueries({ queryKey: ['gen.history'] });
          qc.invalidateQueries({ queryKey: ['billing.logs'] });
        }
      } catch {
        // ignore
      }
    }, 1500);
  };

  const expectedCost = (MODELS.find((m) => m.code === model)?.cost ?? 4) * count;
  const inProgress = task && (task.status === 0 || task.status === 1);
  const results = task?.results ?? [];

  const submitGenerate = () => {
    ensureLoggedIn(() => createMut.mutate(), '登录后即可生成图像');
  };

  return (
    <div
      className="
        grid h-full
        @container
        grid-cols-1
        lg:grid-cols-[clamp(320px,30vw,420px)_1fr]
        2xl:grid-cols-[clamp(320px,26vw,420px)_1fr_clamp(260px,18vw,320px)]
      "
    >
      {/* 左：参数面板 */}
      <section className="border-r border-border bg-surface-1 p-5 lg:p-6 overflow-y-auto">
        <header className="mb-5">
          <h2 className="text-h3 text-text-primary">图像创作</h2>
          <p className="text-small text-text-tertiary mt-1">配置模型、提示词与输出参数。</p>
        </header>

        <ScrollPicker
          label="模型"
          value={model}
          options={MODELS.map((m) => ({
            value: m.code,
            label: m.name,
            badge: m.hot ? '热门' : m.pro ? 'Pro' : undefined,
            cost: m.cost,
          }))}
          onChange={setModel}
        />

        <FieldBlock label="提示词" hint={`${prompt.length}/4000`}>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            rows={6}
            className="textarea leading-loose"
            placeholder="描述你想要的画面，越具体越好"
            maxLength={4000}
          />
          <div className="mt-2 flex flex-wrap gap-1.5">
            {['电影感', '微距摄影', '梦幻光影', '高级感'].map((t) => (
              <button
                key={t}
                type="button"
                className="chip"
                onClick={() => setPrompt((p) => `${p}, ${t}`)}
              >
                + {t}
              </button>
            ))}
          </div>
        </FieldBlock>

        <FieldBlock label="参考图（暂不可用）">
          <button
            className="w-full h-28 grid place-items-center rounded-md border-2 border-dashed border-border opacity-60 cursor-not-allowed"
            disabled
          >
            <div className="flex flex-col items-center gap-1 text-text-tertiary">
              <Upload size={20} />
              <span className="text-small">即将上线</span>
            </div>
          </button>
        </FieldBlock>

        <div className="grid grid-cols-2 gap-4">
          <FieldBlock label="比例">
            <div className="grid grid-cols-5 gap-1.5">
              {RATIOS.map((r) => (
                <Pill key={r} active={r === ratio} onClick={() => setRatio(r)}>
                  {r}
                </Pill>
              ))}
            </div>
          </FieldBlock>
          <FieldBlock label="数量">
            <div className="grid grid-cols-3 gap-1.5">
              {COUNTS.map((c) => (
                <Pill key={c} active={c === count} onClick={() => setCount(c)}>
                  {c}
                </Pill>
              ))}
            </div>
          </FieldBlock>
        </div>

        <FieldBlock label="质量">
          <div className="grid grid-cols-2 gap-1.5">
            {QUALITY.map((q) => (
              <Pill key={q.value} active={q.value === quality} onClick={() => setQuality(q.value)}>
                {q.label}
              </Pill>
            ))}
          </div>
        </FieldBlock>

        <div className="sticky bottom-0 -mx-5 lg:-mx-6 mt-6 px-5 lg:px-6 pt-4 pb-[max(16px,env(safe-area-inset-bottom))] bg-surface-1/95 backdrop-blur border-t border-border">
          <div className="flex items-center justify-between mb-2 text-small">
            <span className="text-text-secondary">预计消耗</span>
            <span className="font-semibold text-klein-500">{expectedCost} 点</span>
          </div>
          <button
            className="btn btn-primary btn-xl btn-block"
            onClick={submitGenerate}
            disabled={createMut.isPending || !!inProgress || !prompt.trim()}
          >
            {createMut.isPending || inProgress ? (
              <><Loader2 size={18} className="animate-spin" />生成中…</>
            ) : (
              <><Sparkles size={18} />立即生成</>
            )}
          </button>
        </div>
      </section>

      {/* 中：结果展示 */}
      <section className="bg-surface-bg overflow-y-auto">
        <div className="p-5 lg:p-8">
          <header className="flex flex-wrap items-center justify-between gap-3 mb-6">
            <div className="flex items-center gap-3 flex-wrap">
              <h3 className="text-h2 text-text-primary">作品预览</h3>
              {task && (
                <div className="2xl:hidden chip chip-outline">
                  <span>{STATUS_LABELS[task.status]}</span>
                  <span className="font-semibold text-klein-500">{task.progress ?? 0}%</span>
                </div>
              )}
            </div>
            <div className="flex items-center gap-2">
              <button
                className="btn btn-outline btn-md"
                onClick={submitGenerate}
                disabled={createMut.isPending || !!inProgress}
              >
                <RefreshCw size={16} /> 再来一组
              </button>
            </div>
          </header>

          <div className="grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(280px,100%),1fr))]">
            {results.length > 0
              ? results.map((r, i) => (
                  <article
                    key={r.url}
                    className="group relative overflow-hidden rounded-lg bg-surface-2 shadow-2 hover:shadow-3 transition"
                    style={{ aspectRatio: ratio.replace(':', '/') }}
                  >
                    <img
                      src={r.url}
                      alt={`生成结果 ${i + 1}`}
                      loading="lazy"
                      className="absolute inset-0 h-full w-full object-cover"
                    />
                    <div className="absolute inset-x-0 bottom-0 p-3 bg-gradient-to-t from-black/60 to-transparent opacity-0 group-hover:opacity-100 transition flex items-center justify-end gap-2">
                      <button className="btn btn-icon btn-sm bg-white/15 text-white hover:bg-white/25" title="收藏">
                        <Heart size={16} />
                      </button>
                      <a
                        className="btn btn-icon btn-sm bg-white/15 text-white hover:bg-white/25"
                        href={r.url}
                        download
                        target="_blank"
                        rel="noreferrer"
                      >
                        <Download size={16} />
                      </a>
                    </div>
                  </article>
                ))
              : Array.from({ length: count }).map((_, i) => (
                  <article
                    key={i}
                    className="relative overflow-hidden rounded-lg bg-klein-gradient-soft border border-border grid place-items-center"
                    style={{ aspectRatio: ratio.replace(':', '/') }}
                  >
                    <span className="text-text-tertiary text-small">
                      {inProgress ? `生成中 ${task?.progress ?? 0}%` : '等待生成'}
                    </span>
                  </article>
                ))}
          </div>

          <div className="mt-10 card card-section">
            <h4 className="section-title mb-2">提示</h4>
            <p className="text-body text-text-secondary leading-loose">
              生成结果将自动保存到「生成历史」，可在 14 天内重新下载或生成变体。
              支持 OpenAI 兼容协议直接调用，参考「调用说明」。
            </p>
          </div>
        </div>
      </section>

      {/* 右：当前任务进度（≥1536px） */}
      <aside className="hidden 2xl:flex flex-col border-l border-border bg-surface-1 overflow-y-auto">
        <div className="p-5">
          <h4 className="section-title mb-3">当前任务</h4>
          {task ? (
            <div className="card-flat p-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-small text-text-secondary">{STATUS_LABELS[task.status]}</span>
                <span className="text-small font-semibold text-klein-500">{task.progress ?? 0}%</span>
              </div>
              <div className="progress">
                <div className="progress-bar" style={{ width: `${task.progress ?? 0}%` }} />
              </div>
              <p className="mt-3 text-small text-text-tertiary leading-loose line-clamp-3">{prompt}</p>
              <p className="mt-2 text-tiny text-text-tertiary">
                Task ID: <code className="font-mono">{task.task_id}</code>
              </p>
              <p className="mt-1 text-tiny text-text-tertiary">
                消耗：{fmtPoints(task.cost_points)} 点
              </p>
            </div>
          ) : (
            <div className="rounded-md border border-dashed border-border p-4 text-text-tertiary text-small">
              点击「立即生成」开始你的创作
            </div>
          )}
        </div>
      </aside>
    </div>
  );
}

/* —— 小组件 —— */

function FieldBlock({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="mb-5">
      <div className="flex items-center justify-between mb-1.5">
        <label className="field-label">{label}</label>
        {hint && <span className="text-tiny text-text-tertiary">{hint}</span>}
      </div>
      {children}
    </div>
  );
}

function Pill({
  active,
  children,
  onClick,
  disabled,
}: {
  active?: boolean;
  children: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={clsx(
        'h-9 min-w-[44px] px-3 rounded-md text-small font-medium border transition',
        active
          ? 'bg-klein-gradient text-text-on-klein border-transparent shadow-glow-soft'
          : 'border-border text-text-secondary hover:text-text-primary hover:border-klein-500',
        disabled && 'opacity-50 cursor-not-allowed',
      )}
    >
      {children}
    </button>
  );
}

function ScrollPicker({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: { value: string; label: string; badge?: string; cost?: number }[];
  onChange: (v: string) => void;
}) {
  return (
    <div className="mb-5">
      <label className="field-label mb-1.5 block">{label}</label>
      <div className="grid grid-cols-2 gap-2">
        {options.map((o) => {
          const active = o.value === value;
          return (
            <button
              key={o.value}
              type="button"
              onClick={() => onChange(o.value)}
              className={clsx(
                'relative flex flex-col items-start p-3 rounded-md border text-left transition',
                active
                  ? 'border-klein-500 bg-klein-gradient-soft shadow-glow-soft'
                  : 'border-border hover:border-klein-300',
              )}
            >
              <span className="font-medium text-text-primary">{o.label}</span>
              {o.cost !== undefined && (
                <span className="text-small text-text-tertiary mt-0.5">{o.cost} 点 / 张</span>
              )}
              {o.badge && (
                <span className="absolute top-2 right-2 badge badge-klein">{o.badge}</span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
