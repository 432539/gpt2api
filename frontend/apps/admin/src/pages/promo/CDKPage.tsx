import { useMutation } from '@tanstack/react-query';
import { Ticket, AlertCircle, CheckCircle2 } from 'lucide-react';
import { useState } from 'react';

import { ApiError } from '../../lib/api';
import { cdkApi } from '../../lib/services';
import type { CDKCreateBatchBody, CDKCreateBatchResp } from '../../lib/types';
import { fmtNumber, fmtPoints } from '../../lib/format';
import { toast } from '../../stores/toast';

export default function CDKPage() {
  const [body, setBody] = useState<CDKCreateBatchBody>({
    batch_no: '',
    name: '',
    points: 1000, // 后端 *100，1000 = 10 点
    qty: 100,
    per_user_limit: 1,
    expire_at: 0,
  });
  const [last, setLast] = useState<CDKCreateBatchResp | null>(null);

  const m = useMutation({
    mutationFn: (b: CDKCreateBatchBody) => cdkApi.createBatch(b),
    onSuccess: (r) => {
      toast.success(`已生成批次 ${r.batch_no}（共 ${r.total_qty} 张）`);
      setLast(r);
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!body.batch_no.trim() || !body.name.trim()) {
      toast.error('请填写批次号和名称');
      return;
    }
    if (body.points <= 0 || body.qty <= 0) {
      toast.error('点数和数量必须 > 0');
      return;
    }
    m.mutate({
      ...body,
      batch_no: body.batch_no.trim(),
      name: body.name.trim(),
      per_user_limit: body.per_user_limit || 0,
      expire_at: body.expire_at || undefined,
    });
  };

  return (
    <div className="page page-wide space-y-6">
      <header className="page-header">
        <div>
          <h1 className="page-title flex items-center gap-2">
            <Ticket className="text-klein-500" size={26} />
            兑换码 CDK
          </h1>
          <p className="page-subtitle">
            按批次生成；每张 CDK 只能被使用一次，使用后写入 wallet_log 并入账。
          </p>
        </div>
      </header>

      <form onSubmit={submit} className="card card-section grid lg:grid-cols-2 gap-5 max-w-4xl">
        <Field label="批次号" hint="同批次唯一，如 SPRING2026-A">
          <input
            className="input"
            value={body.batch_no}
            onChange={(e) => setBody((s) => ({ ...s, batch_no: e.target.value }))}
            placeholder="SPRING2026-A"
          />
        </Field>

        <Field label="批次名称" hint="展示给运营 / 客服的友好名称">
          <input
            className="input"
            value={body.name}
            onChange={(e) => setBody((s) => ({ ...s, name: e.target.value }))}
            placeholder="春节活动 100 点"
          />
        </Field>

        <Field
          label="单码点数（×100 储存）"
          hint={`输入 1000 = 实际 10.00 点；当前等价：${fmtPoints(body.points)} 点`}
        >
          <input
            type="number"
            min={1}
            className="input"
            value={body.points}
            onChange={(e) =>
              setBody((s) => ({ ...s, points: Math.max(1, Number(e.target.value) || 0) }))
            }
          />
        </Field>

        <Field label="生成数量" hint="单批次最多 100,000 张">
          <input
            type="number"
            min={1}
            max={100_000}
            className="input"
            value={body.qty}
            onChange={(e) =>
              setBody((s) => ({ ...s, qty: Math.max(1, Number(e.target.value) || 0) }))
            }
          />
        </Field>

        <Field label="每用户限领次数" hint="0 表示不限制；建议 1（防止羊毛党）">
          <input
            type="number"
            min={0}
            className="input"
            value={body.per_user_limit ?? 0}
            onChange={(e) => setBody((s) => ({ ...s, per_user_limit: Number(e.target.value) || 0 }))}
          />
        </Field>

        <Field label="过期时间（可选）" hint="留空表示永久有效">
          <input
            type="datetime-local"
            className="input"
            onChange={(e) => {
              const v = e.target.value;
              if (!v) {
                setBody((s) => ({ ...s, expire_at: 0 }));
                return;
              }
              const t = Math.floor(new Date(v).getTime() / 1000);
              setBody((s) => ({ ...s, expire_at: t }));
            }}
          />
        </Field>

        <div className="lg:col-span-2 flex flex-wrap items-center justify-between gap-3 rounded-md bg-klein-gradient-soft p-4">
          <div className="flex items-center gap-2 text-small text-text-secondary">
            <AlertCircle size={16} className="text-klein-500" />
            预计生成：
            <strong className="text-text-primary mx-1">{fmtNumber(body.qty)}</strong>
            张，单码价值
            <strong className="text-text-primary mx-1">{fmtPoints(body.points)} 点</strong>，
            合计
            <strong className="text-klein-500 mx-1">{fmtPoints(body.points * body.qty)} 点</strong>
          </div>
          <button type="submit" className="btn btn-primary btn-md" disabled={m.isPending}>
            {m.isPending ? '生成中…' : '生成批次'}
          </button>
        </div>
      </form>

      {last && (
        <div className="card card-section flex items-start gap-3 border-success/40 max-w-4xl">
          <CheckCircle2 className="text-success shrink-0 mt-0.5" size={20} />
          <div className="flex-1 space-y-1">
            <p className="text-text-primary font-medium">最新生成成功</p>
            <p className="text-small text-text-secondary">
              批次 ID #{last.id} · 批次号
              <code className="kbd mx-1">{last.batch_no}</code>
              · 共 {fmtNumber(last.total_qty)} 张
            </p>
            <p className="text-small text-text-tertiary">
              详细码列表请前往 DB / 下一阶段补全的 CDK 列表 UI 导出（CSV）。
            </p>
          </div>
        </div>
      )}
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
