import { useEffect, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import clsx from 'clsx';
import { Loader2, Video, Wand2 } from 'lucide-react';

import { useEnsureLoggedIn } from '../../hooks/useEnsureLoggedIn';
import { ApiError } from '../../lib/api';
import { fmtPoints } from '../../lib/format';
import { genApi } from '../../lib/services';
import type { GenerationTask } from '../../lib/types';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

const VIDEO_MODELS = [
  { code: 'vid-v1',  name: 'GROK 文生视频 V1', cost: 15 },
  { code: 'vid-i2v', name: 'GROK 图生视频 V1', cost: 20 },
];

const DURATIONS = [4, 8, 16] as const;
const RATIOS = ['16:9', '9:16', '1:1'] as const;
const QUALITY = [
  { value: 'standard', label: '标准' },
  { value: 'hd', label: '高清' },
] as const;

type Quality = (typeof QUALITY)[number]['value'];

export default function CreateVideoPage() {
  const qc = useQueryClient();
  const refreshMe = useAuthStore((s) => s.refreshMe);
  const ensureLoggedIn = useEnsureLoggedIn();

  const [model, setModel] = useState(VIDEO_MODELS[0]!.code);
  const [duration, setDuration] = useState<(typeof DURATIONS)[number]>(8);
  const [ratio, setRatio] = useState<(typeof RATIOS)[number]>('16:9');
  const [quality, setQuality] = useState<Quality>('hd');
  const [prompt, setPrompt] = useState('一只小猫穿过霓虹小巷，慢镜头，电影感');

  const [task, setTask] = useState<GenerationTask | null>(null);
  const pollRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current);
    };
  }, []);

  const createMut = useMutation({
    mutationFn: () => genApi.createVideo({ model, prompt, duration, ratio, quality }),
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
          if (fresh.status === 2) toast.success('视频生成完成');
          else if (fresh.status === 3) toast.error(fresh.error || '生成失败');
          else toast.info('生成失败已退款');
          await refreshMe();
          qc.invalidateQueries({ queryKey: ['gen.history'] });
          qc.invalidateQueries({ queryKey: ['billing.logs'] });
        }
      } catch {
        // ignore
      }
    }, 2000);
  };

  const baseCost = VIDEO_MODELS.find((m) => m.code === model)?.cost ?? 15;
  const expectedCost = duration > 4 ? Math.round((baseCost * duration) / 4) : baseCost;
  const inProgress = task && (task.status === 0 || task.status === 1);
  const result = task?.results?.[0];

  const submitGenerate = () => {
    ensureLoggedIn(() => createMut.mutate(), '登录后即可生成视频');
  };

  return (
    <div className="page">
      <header className="page-header">
        <div className="flex items-center gap-3">
          <div className="grid place-items-center w-11 h-11 rounded-md bg-klein-gradient text-text-on-klein shadow-glow-soft flex-shrink-0">
            <Video size={22} />
          </div>
          <div className="min-w-0">
            <h1 className="page-title">视频创作 · GROK</h1>
            <p className="page-subtitle">支持文本生视频 / 图生视频 · 4-16 秒</p>
          </div>
        </div>
      </header>

      <div className="grid lg:grid-cols-2 gap-5">
        <section className="card card-section space-y-5">
          <div className="field">
            <label className="field-label">模型</label>
            <div className="grid grid-cols-2 gap-2">
              {VIDEO_MODELS.map((m) => (
                <button
                  key={m.code}
                  type="button"
                  className={clsx(
                    'flex flex-col items-start p-3 rounded-md border text-left transition',
                    m.code === model
                      ? 'border-klein-500 bg-klein-gradient-soft shadow-glow-soft'
                      : 'border-border hover:border-klein-300',
                  )}
                  onClick={() => setModel(m.code)}
                >
                  <span className="font-medium text-text-primary">{m.name}</span>
                  <span className="text-small text-text-tertiary mt-0.5">{m.cost} 点 / 4 秒</span>
                </button>
              ))}
            </div>
          </div>

          <div className="field">
            <div className="flex items-center justify-between">
              <label className="field-label">文本描述</label>
              <span className="text-tiny text-text-tertiary">{prompt.length}/4000</span>
            </div>
            <textarea
              rows={5}
              className="textarea leading-loose"
              placeholder="例：一只小猫穿过霓虹小巷，慢镜头，电影感"
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              maxLength={4000}
            />
          </div>

          <div className="grid grid-cols-3 gap-3">
            <Selector
              label="时长（秒）"
              options={DURATIONS.map((d) => ({ value: String(d), label: `${d}s` }))}
              active={String(duration)}
              onChange={(v) => setDuration(Number(v) as (typeof DURATIONS)[number])}
            />
            <Selector
              label="比例"
              options={RATIOS.map((r) => ({ value: r, label: r }))}
              active={ratio}
              onChange={(v) => setRatio(v as (typeof RATIOS)[number])}
            />
            <Selector
              label="质量"
              options={QUALITY.map((q) => ({ value: q.value, label: q.label }))}
              active={quality}
              onChange={(v) => setQuality(v as Quality)}
            />
          </div>

          <div className="flex items-center justify-between pt-4 border-t border-border">
            <span className="text-small text-text-secondary">
              预计消耗 <span className="text-klein-500 font-bold">{expectedCost}</span> 点
            </span>
            <button
              className="btn btn-primary btn-lg"
              onClick={submitGenerate}
              disabled={createMut.isPending || !!inProgress || !prompt.trim()}
            >
              {createMut.isPending || inProgress ? (
                <><Loader2 size={18} className="animate-spin" />生成中…</>
              ) : (
                <><Wand2 size={18} />生成视频</>
              )}
            </button>
          </div>
        </section>

        <section className="card card-section">
          <h3 className="section-title mb-3">实时预览</h3>
          <div
            className="rounded-md bg-klein-gradient-soft grid place-items-center text-text-tertiary overflow-hidden border border-border"
            style={{ aspectRatio: ratio.replace(':', '/') }}
          >
            {result?.url ? (
              <video src={result.url} controls playsInline className="h-full w-full object-cover" />
            ) : inProgress ? (
              <div className="flex flex-col items-center gap-2">
                <Loader2 size={28} className="animate-spin text-klein-500" />
                <span className="text-small">{task?.progress ?? 0}%</span>
              </div>
            ) : (
              <span className="text-small">生成完成后会自动显示预览</span>
            )}
          </div>
          {task && (
            <div className="mt-4 text-small text-text-tertiary space-y-1">
              <p>Task: <code className="font-mono">{task.task_id}</code></p>
              <p>消耗：{fmtPoints(task.cost_points)} 点</p>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

function Selector({
  label,
  options,
  active,
  onChange,
}: {
  label: string;
  options: { value: string; label: string }[];
  active: string;
  onChange: (v: string) => void;
}) {
  return (
    <div className="field">
      <label className="field-label">{label}</label>
      <div className="grid grid-cols-3 gap-1.5">
        {options.map((o) => (
          <button
            key={o.value}
            type="button"
            onClick={() => onChange(o.value)}
            className={clsx(
              'h-9 rounded-md text-small font-medium transition',
              o.value === active
                ? 'bg-klein-gradient text-text-on-klein shadow-glow-soft'
                : 'border border-border text-text-secondary hover:border-klein-500',
            )}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  );
}
