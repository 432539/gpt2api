import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, RefreshCw, Save, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';

import { ApiError } from '../../lib/api';
import { systemApi } from '../../lib/services';
import { toast } from '../../stores/toast';

interface PriceRow {
  model_code: string;
  name: string;
  kind: 'text' | 'image' | 'video';
  provider: 'gpt' | 'grok' | string;
  upstream_model: string;
  unit_points: number;
  input_unit_points?: number;
  output_unit_points?: number;
  enabled: boolean;
}

const DEFAULT_ROWS: PriceRow[] = [
  { model_code: 'gpt-4o-mini', name: '文字对话', kind: 'text', provider: 'gpt', upstream_model: 'gpt-4o-mini', unit_points: 0, input_unit_points: 1, output_unit_points: 3, enabled: true },
  { model_code: 'img-v3', name: '通用图片', kind: 'image', provider: 'gpt', upstream_model: 'gpt-image', unit_points: 4, enabled: true },
  { model_code: 'img-real', name: '真实图片', kind: 'image', provider: 'gpt', upstream_model: 'gpt-image-real', unit_points: 4, enabled: true },
  { model_code: 'img-anime', name: '动漫图片', kind: 'image', provider: 'gpt', upstream_model: 'gpt-image-anime', unit_points: 3, enabled: true },
  { model_code: 'img-3d', name: '3D 图片', kind: 'image', provider: 'gpt', upstream_model: 'gpt-image-3d', unit_points: 5, enabled: true },
  { model_code: 'vid-v1', name: '视频生成', kind: 'video', provider: 'grok', upstream_model: 'grok-video', unit_points: 15, enabled: true },
  { model_code: 'vid-i2v', name: '图生视频', kind: 'video', provider: 'grok', upstream_model: 'grok-i2v', unit_points: 20, enabled: true },
];

function fromValue(v: unknown): PriceRow[] {
  if (Array.isArray(v)) {
    return v.map((r) => {
      const row = r as Partial<PriceRow>;
      return {
        model_code: String(row.model_code || ''),
        name: String(row.name || ''),
        kind: row.kind === 'text' ? 'text' : row.kind === 'video' ? 'video' : 'image',
        provider: String(row.provider || 'gpt'),
        upstream_model: String(row.upstream_model || ''),
        unit_points: Number(row.unit_points || 0) / 100,
        input_unit_points: Number(row.input_unit_points || 0) / 100,
        output_unit_points: Number(row.output_unit_points || 0) / 100,
        enabled: row.enabled !== false,
      };
    });
  }
  if (v && typeof v === 'object') {
    return Object.entries(v as Record<string, number>).map(([model_code, price]) => ({
      model_code,
      name: model_code,
      kind: model_code.startsWith('vid') ? 'video' : model_code.startsWith('gpt') ? 'text' : 'image',
      provider: model_code.startsWith('vid') ? 'grok' : 'gpt',
      upstream_model: model_code,
      unit_points: Number(price || 0) / 100,
      input_unit_points: model_code.startsWith('gpt') ? Number(price || 0) / 100 : 0,
      output_unit_points: model_code.startsWith('gpt') ? Number(price || 0) / 100 : 0,
      enabled: true,
    }));
  }
  return DEFAULT_ROWS;
}

export default function ModelPricesPage() {
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ['admin', 'system', 'settings'], queryFn: () => systemApi.get() });
  const [rows, setRows] = useState<PriceRow[]>(DEFAULT_ROWS);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (settings.data) {
      setRows(fromValue(settings.data['billing.model_prices']));
      setDirty(false);
    }
  }, [settings.data]);

  const update = (idx: number, patch: Partial<PriceRow>) => {
    setRows((old) => old.map((row, i) => (i === idx ? { ...row, ...patch } : row)));
    setDirty(true);
  };

  const save = useMutation({
    mutationFn: () => systemApi.update({
      'billing.model_prices': rows.map((row) => ({
        ...row,
        model_code: row.model_code.trim(),
        name: row.name.trim(),
        provider: row.provider.trim(),
        upstream_model: row.upstream_model.trim(),
        unit_points: Math.round((Number(row.unit_points) || 0) * 100),
        input_unit_points: Math.round((Number(row.input_unit_points) || 0) * 100),
        output_unit_points: Math.round((Number(row.output_unit_points) || 0) * 100),
      })),
    }),
    onSuccess: () => {
      toast.success('模型价格已保存');
      setDirty(false);
      qc.invalidateQueries({ queryKey: ['admin', 'system'] });
    },
    onError: (e: ApiError) => toast.error(e.message),
  });

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">模型价格</h1>
          <p className="page-subtitle">维护模型编码、上游模型映射、供应商和扣费单价</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-outline btn-md" onClick={() => settings.refetch()} disabled={settings.isFetching}>
            <RefreshCw size={16} className={settings.isFetching ? 'animate-spin' : ''} /> 重新加载
          </button>
          <button className="btn btn-primary btn-md" onClick={() => save.mutate()} disabled={!dirty || save.isPending}>
            <Save size={16} /> {save.isPending ? '保存中…' : dirty ? '保存修改' : '已是最新'}
          </button>
        </div>
      </header>

      <div className="card table-wrap">
        <table className="data-table">
          <thead>
            <tr>
              <th>模型编码</th>
              <th>显示名称</th>
              <th>类型</th>
              <th>供应商</th>
              <th>上游模型映射</th>
              <th>单价（点）</th>
              <th>输入/输出（点/千Token）</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, idx) => (
              <tr key={`${row.model_code}-${idx}`}>
                <td><input className="input min-w-[130px]" value={row.model_code} onChange={(e) => update(idx, { model_code: e.target.value })} /></td>
                <td><input className="input min-w-[130px]" value={row.name} onChange={(e) => update(idx, { name: e.target.value })} /></td>
                <td>
                  <select className="input min-w-[96px]" value={row.kind} onChange={(e) => update(idx, { kind: e.target.value as PriceRow['kind'] })}>
                    <option value="text">文字</option>
                    <option value="image">图片</option>
                    <option value="video">视频</option>
                  </select>
                </td>
                <td><input className="input min-w-[90px]" value={row.provider} onChange={(e) => update(idx, { provider: e.target.value })} /></td>
                <td><input className="input min-w-[150px]" value={row.upstream_model} onChange={(e) => update(idx, { upstream_model: e.target.value })} /></td>
                <td><input className="input w-[100px]" type="number" min={0} value={row.unit_points} onChange={(e) => update(idx, { unit_points: Number(e.target.value) || 0 })} disabled={row.kind === 'text'} /></td>
                <td>
                  {row.kind === 'text' ? (
                    <div className="flex gap-2">
                      <input className="input w-[90px]" type="number" min={0} value={row.input_unit_points || 0} onChange={(e) => update(idx, { input_unit_points: Number(e.target.value) || 0 })} placeholder="输入" />
                      <input className="input w-[90px]" type="number" min={0} value={row.output_unit_points || 0} onChange={(e) => update(idx, { output_unit_points: Number(e.target.value) || 0 })} placeholder="输出" />
                    </div>
                  ) : (
                    <span className="text-muted">-</span>
                  )}
                </td>
                <td>
                  <button className={row.enabled ? 'btn btn-outline btn-sm' : 'btn btn-ghost btn-sm'} onClick={() => update(idx, { enabled: !row.enabled })}>
                    {row.enabled ? '启用' : '停用'}
                  </button>
                </td>
                <td>
                  <button className="btn btn-danger-ghost btn-icon btn-sm" onClick={() => { setRows((old) => old.filter((_, i) => i !== idx)); setDirty(true); }}>
                    <Trash2 size={14} />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <button
        className="btn btn-outline btn-md"
        onClick={() => {
          setRows((old) => [...old, { model_code: '', name: '', kind: 'image', provider: 'gpt', upstream_model: '', unit_points: 0, input_unit_points: 0, output_unit_points: 0, enabled: true }]);
          setDirty(true);
        }}
      >
        <Plus size={16} /> 添加模型
      </button>
    </div>
  );
}
