import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Cloud, CreditCard, Database, RefreshCw, Save, ShieldAlert, Trash2 } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';

import { ApiError } from '../../lib/api';
import { proxiesApi, systemApi } from '../../lib/services';
import type { ProxyItem, SystemSettings } from '../../lib/types';
import { toast } from '../../stores/toast';

interface FormState {
  retry_max_attempts: number;
  retry_base_delay_ms: number;
  retry_timeout_seconds: number;
  tolerance_circuit_failures: number;
  tolerance_circuit_cooldown_seconds: number;
  proxy_global_enabled: boolean;
  proxy_global_id: number;
  oauth_refresh_before_hours: number;
  storage_history_retention_days: number;
  storage_result_retention_days: number;
  storage_result_cache_driver: string;
  oss_enabled: boolean;
  oss_provider: string;
  oss_endpoint: string;
  oss_region: string;
  oss_bucket: string;
  oss_access_key_id: string;
  oss_access_key_secret: string;
  oss_public_base_url: string;
  oss_path_prefix: string;
  payment_enabled: boolean;
  payment_provider: string;
  payment_notify_url: string;
  alipay_app_id: string;
  alipay_private_key: string;
  wechat_mch_id: string;
  wechat_api_v3_key: string;
}

const DEFAULT_FORM: FormState = {
  retry_max_attempts: 2,
  retry_base_delay_ms: 800,
  retry_timeout_seconds: 300,
  tolerance_circuit_failures: 3,
  tolerance_circuit_cooldown_seconds: 300,
  proxy_global_enabled: false,
  proxy_global_id: 0,
  oauth_refresh_before_hours: 6,
  storage_history_retention_days: 180,
  storage_result_retention_days: 30,
  storage_result_cache_driver: 'local',
  oss_enabled: false,
  oss_provider: 'aliyun',
  oss_endpoint: '',
  oss_region: '',
  oss_bucket: '',
  oss_access_key_id: '',
  oss_access_key_secret: '',
  oss_public_base_url: '',
  oss_path_prefix: 'uploads/{yyyy}/{mm}/{dd}',
  payment_enabled: false,
  payment_provider: 'alipay',
  payment_notify_url: '',
  alipay_app_id: '',
  alipay_private_key: '',
  wechat_mch_id: '',
  wechat_api_v3_key: '',
};

const asBool = (v: unknown, fallback = false) => (v == null ? fallback : Boolean(v));
const asNum = (v: unknown, fallback: number) => {
  const n = Number(v);
  return Number.isFinite(n) ? n : fallback;
};
const asStr = (v: unknown, fallback = '') => (typeof v === 'string' ? v : fallback);

function fromSettings(s: SystemSettings | undefined): FormState {
  if (!s) return DEFAULT_FORM;
  return {
    retry_max_attempts: asNum(s['retry.max_attempts'], 2),
    retry_base_delay_ms: asNum(s['retry.base_delay_ms'], 800),
    retry_timeout_seconds: asNum(s['retry.timeout_seconds'], 300),
    tolerance_circuit_failures: asNum(s['tolerance.circuit_failures'], 3),
    tolerance_circuit_cooldown_seconds: asNum(s['tolerance.circuit_cooldown_seconds'], 300),
    proxy_global_enabled: asBool(s['proxy.global_enabled']),
    proxy_global_id: asNum(s['proxy.global_id'], 0),
    oauth_refresh_before_hours: asNum(s['oauth.refresh_before_hours'], 6),
    storage_history_retention_days: asNum(s['storage.history_retention_days'], 180),
    storage_result_retention_days: asNum(s['storage.result_retention_days'], 30),
    storage_result_cache_driver: asStr(s['storage.result_cache_driver'], 'local'),
    oss_enabled: asBool(s['oss.enabled']),
    oss_provider: asStr(s['oss.provider'], 'aliyun'),
    oss_endpoint: asStr(s['oss.endpoint']),
    oss_region: asStr(s['oss.region']),
    oss_bucket: asStr(s['oss.bucket']),
    oss_access_key_id: asStr(s['oss.access_key_id']),
    oss_access_key_secret: asStr(s['oss.access_key_secret']),
    oss_public_base_url: asStr(s['oss.public_base_url']),
    oss_path_prefix: asStr(s['oss.path_prefix'], 'uploads/{yyyy}/{mm}/{dd}'),
    payment_enabled: asBool(s['payment.enabled']),
    payment_provider: asStr(s['payment.provider'], 'alipay'),
    payment_notify_url: asStr(s['payment.notify_url']),
    alipay_app_id: asStr(s['payment.alipay_app_id']),
    alipay_private_key: asStr(s['payment.alipay_private_key']),
    wechat_mch_id: asStr(s['payment.wechat_mch_id']),
    wechat_api_v3_key: asStr(s['payment.wechat_api_v3_key']),
  };
}

function toPayload(f: FormState): Partial<SystemSettings> {
  return {
    'retry.max_attempts': Number(f.retry_max_attempts) || 0,
    'retry.base_delay_ms': Number(f.retry_base_delay_ms) || 0,
    'retry.timeout_seconds': Number(f.retry_timeout_seconds) || 0,
    'tolerance.circuit_failures': Number(f.tolerance_circuit_failures) || 0,
    'tolerance.circuit_cooldown_seconds': Number(f.tolerance_circuit_cooldown_seconds) || 0,
    'proxy.global_enabled': f.proxy_global_enabled,
    'proxy.global_id': Number(f.proxy_global_id) || 0,
    'oauth.refresh_before_hours': Number(f.oauth_refresh_before_hours) || 6,
    'storage.history_retention_days': Number(f.storage_history_retention_days) || 0,
    'storage.result_retention_days': Number(f.storage_result_retention_days) || 0,
    'storage.result_cache_driver': f.storage_result_cache_driver,
    'oss.enabled': f.oss_enabled,
    'oss.provider': f.oss_provider.trim(),
    'oss.endpoint': f.oss_endpoint.trim(),
    'oss.region': f.oss_region.trim(),
    'oss.bucket': f.oss_bucket.trim(),
    'oss.access_key_id': f.oss_access_key_id.trim(),
    'oss.access_key_secret': f.oss_access_key_secret.trim(),
    'oss.public_base_url': f.oss_public_base_url.trim(),
    'oss.path_prefix': f.oss_path_prefix.trim(),
    'payment.enabled': f.payment_enabled,
    'payment.provider': f.payment_provider.trim(),
    'payment.notify_url': f.payment_notify_url.trim(),
    'payment.alipay_app_id': f.alipay_app_id.trim(),
    'payment.alipay_private_key': f.alipay_private_key.trim(),
    'payment.wechat_mch_id': f.wechat_mch_id.trim(),
    'payment.wechat_api_v3_key': f.wechat_api_v3_key.trim(),
  };
}

export default function ConfigPage() {
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ['admin', 'system', 'settings'], queryFn: () => systemApi.get() });
  const cacheStats = useQuery({ queryKey: ['admin', 'system', 'cache'], queryFn: () => systemApi.cacheStats() });
  const proxies = useQuery({
    queryKey: ['admin', 'proxies', 'options'],
    queryFn: () => proxiesApi.list({ page: 1, page_size: 200, status: 1 }),
  });
  const [form, setForm] = useState<FormState>(DEFAULT_FORM);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (settings.data) {
      setForm(fromSettings(settings.data));
      setDirty(false);
    }
  }, [settings.data]);

  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => {
    setForm((f) => ({ ...f, [k]: v }));
    setDirty(true);
  };

  const save = useMutation({
    mutationFn: () => systemApi.update(toPayload(form)),
    onSuccess: () => {
      toast.success('已保存');
      setDirty(false);
      qc.invalidateQueries({ queryKey: ['admin', 'system'] });
    },
    onError: (e: ApiError | Error) => toast.error(e.message),
  });

  const cleanCache = useMutation({
    mutationFn: (body: { days?: number; all?: boolean }) => systemApi.cleanCache(body),
    onSuccess: (r) => {
      toast.success(`已清理 ${formatBytes(r.deleted_bytes)} / ${r.deleted_files} 个缓存文件`);
      qc.invalidateQueries({ queryKey: ['admin', 'system', 'cache'] });
    },
    onError: (e: ApiError | Error) => toast.error(e.message),
  });

  const proxyOptions: ProxyItem[] = proxies.data?.list ?? [];

  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">系统配置</h1>
          <p className="page-subtitle">维护运行容错、刷新存储、OSS 和支付通道基础参数。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-outline btn-md" onClick={() => settings.refetch()} disabled={settings.isFetching}>
            <RefreshCw size={16} className={settings.isFetching ? 'animate-spin' : ''} /> 重新加载
          </button>
          <button className="btn btn-primary btn-md" onClick={() => save.mutate()} disabled={!dirty || save.isPending}>
            <Save size={16} /> {save.isPending ? '保存中...' : dirty ? '保存修改' : '已是最新'}
          </button>
        </div>
      </header>

      {settings.isLoading ? (
        <div className="card card-section text-center text-text-tertiary py-10">加载中...</div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          <Section icon={<ShieldAlert size={18} />} title="重试与容错" desc="控制生成请求失败后的重试次数、超时和账号熔断策略。">
            <NumberField label="最大重试次数" value={form.retry_max_attempts} min={0} max={10} onChange={(v) => set('retry_max_attempts', v)} />
            <NumberField label="重试基础延迟（毫秒）" value={form.retry_base_delay_ms} min={0} onChange={(v) => set('retry_base_delay_ms', v)} />
            <NumberField label="请求超时（秒）" value={form.retry_timeout_seconds} min={30} onChange={(v) => set('retry_timeout_seconds', v)} />
            <NumberField label="熔断失败次数" value={form.tolerance_circuit_failures} min={1} onChange={(v) => set('tolerance_circuit_failures', v)} />
            <NumberField label="熔断冷却时间（秒）" value={form.tolerance_circuit_cooldown_seconds} min={30} onChange={(v) => set('tolerance_circuit_cooldown_seconds', v)} />
          </Section>

          <Section icon={<Database size={18} />} title="刷新与存储" desc="控制 OAuth 刷新窗口、全局代理和生成历史保留周期。">
            <Toggle label="启用全局代理" checked={form.proxy_global_enabled} onChange={(v) => set('proxy_global_enabled', v)} />
            <Field label="全局默认代理">
              <select className="select" value={form.proxy_global_id} onChange={(e) => set('proxy_global_id', Number(e.target.value) || 0)} disabled={!form.proxy_global_enabled}>
                <option value={0}>不指定</option>
                {proxyOptions.map((p) => <option key={p.id} value={p.id}>[{p.protocol}] {p.name} - {p.host}:{p.port}</option>)}
              </select>
            </Field>
            <NumberField label="OAuth 提前刷新窗口（小时）" value={form.oauth_refresh_before_hours} min={1} max={48} onChange={(v) => set('oauth_refresh_before_hours', v)} />
            <NumberField label="生成历史保留（天）" value={form.storage_history_retention_days} min={0} onChange={(v) => set('storage_history_retention_days', v)} />
            <NumberField label="生成结果文件保留（天）" value={form.storage_result_retention_days} min={0} onChange={(v) => set('storage_result_retention_days', v)} />
            <Field label="生成结果缓存位置">
              <select className="select" value={form.storage_result_cache_driver} onChange={(e) => set('storage_result_cache_driver', e.target.value)}>
                <option value="local">本地缓存</option>
                <option value="oss">OSS 存储</option>
                <option value="off">不缓存</option>
              </select>
            </Field>
          </Section>

          <Section icon={<Trash2 size={18} />} title="缓存清理" desc="查看并清理本地生成结果缓存，清理后旧作品可能无法继续预览原文件。">
            <div className="grid gap-3 md:grid-cols-3">
              <div className="rounded-md border border-border bg-surface-2 p-3">
                <div className="text-small text-text-tertiary">缓存大小</div>
                <div className="mt-1 text-h4 text-text-primary">{formatBytes(cacheStats.data?.bytes ?? 0)}</div>
              </div>
              <div className="rounded-md border border-border bg-surface-2 p-3">
                <div className="text-small text-text-tertiary">文件数量</div>
                <div className="mt-1 text-h4 text-text-primary">{cacheStats.data?.files ?? 0}</div>
              </div>
              <div className="rounded-md border border-border bg-surface-2 p-3">
                <div className="text-small text-text-tertiary">缓存目录</div>
                <div className="mt-1 truncate text-small text-text-secondary" title={cacheStats.data?.root}>{cacheStats.data?.root || '-'}</div>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <button className="btn btn-outline btn-sm" disabled={cleanCache.isPending} onClick={() => cacheStats.refetch()}>
                <RefreshCw size={14} className={cacheStats.isFetching ? 'animate-spin' : ''} /> 刷新占用
              </button>
              <button className="btn btn-outline btn-sm" disabled={cleanCache.isPending} onClick={() => cleanCache.mutate({ days: 7 })}>
                清理 7 天前
              </button>
              <button className="btn btn-outline btn-sm" disabled={cleanCache.isPending} onClick={() => cleanCache.mutate({ days: 3 })}>
                清理 3 天前
              </button>
              <button
                className="btn btn-danger btn-sm"
                disabled={cleanCache.isPending}
                onClick={() => {
                  if (window.confirm('确定清空全部生成缓存吗？旧作品可能无法继续预览原文件。')) cleanCache.mutate({ all: true });
                }}
              >
                <Trash2 size={14} /> 清空全部缓存
              </button>
            </div>
          </Section>

          <Section icon={<Cloud size={18} />} title="OSS 存储" desc="配置图片、视频和用户上传素材的对象存储位置。">
            <Toggle label="启用 OSS 存储" checked={form.oss_enabled} onChange={(v) => set('oss_enabled', v)} />
            <div className="grid gap-3 md:grid-cols-2">
              <TextField label="服务商" value={form.oss_provider} onChange={(v) => set('oss_provider', v)} placeholder="aliyun / s3 / cos" />
              <TextField label="Region" value={form.oss_region} onChange={(v) => set('oss_region', v)} />
              <TextField label="Endpoint" value={form.oss_endpoint} onChange={(v) => set('oss_endpoint', v)} />
              <TextField label="Bucket" value={form.oss_bucket} onChange={(v) => set('oss_bucket', v)} />
              <TextField label="AccessKey ID" value={form.oss_access_key_id} onChange={(v) => set('oss_access_key_id', v)} />
              <TextField label="AccessKey Secret" value={form.oss_access_key_secret} onChange={(v) => set('oss_access_key_secret', v)} type="password" />
            </div>
            <TextField label="公开访问域名" value={form.oss_public_base_url} onChange={(v) => set('oss_public_base_url', v)} placeholder="https://cdn.example.com" />
            <TextField label="存储路径前缀" value={form.oss_path_prefix} onChange={(v) => set('oss_path_prefix', v)} />
          </Section>

          <Section icon={<CreditCard size={18} />} title="支付配置" desc="保存支付通道基础参数，后续充值下单与回调会读取这些配置。">
            <Toggle label="启用在线支付" checked={form.payment_enabled} onChange={(v) => set('payment_enabled', v)} />
            <div className="grid gap-3 md:grid-cols-2">
              <TextField label="默认支付通道" value={form.payment_provider} onChange={(v) => set('payment_provider', v)} placeholder="alipay / wechat" />
              <TextField label="支付回调地址" value={form.payment_notify_url} onChange={(v) => set('payment_notify_url', v)} />
              <TextField label="支付宝 AppID" value={form.alipay_app_id} onChange={(v) => set('alipay_app_id', v)} />
              <TextField label="微信商户号" value={form.wechat_mch_id} onChange={(v) => set('wechat_mch_id', v)} />
            </div>
            <Field label="支付宝私钥"><textarea className="input font-mono text-small min-h-[96px]" value={form.alipay_private_key} onChange={(e) => set('alipay_private_key', e.target.value)} /></Field>
            <TextField label="微信 API v3 Key" value={form.wechat_api_v3_key} onChange={(v) => set('wechat_api_v3_key', v)} type="password" />
          </Section>
        </div>
      )}
    </div>
  );
}

function Section({ icon, title, desc, children }: { icon: ReactNode; title: string; desc: string; children: ReactNode }) {
  return (
    <section className="card card-section space-y-4">
      <header className="flex items-start gap-3">
        <span className="grid place-items-center w-9 h-9 rounded-md bg-info-soft text-klein-500">{icon}</span>
        <div>
          <h2 className="text-h5 font-semibold text-text-primary">{title}</h2>
          <p className="text-small text-text-tertiary mt-0.5">{desc}</p>
        </div>
      </header>
      <div className="grid gap-3">{children}</div>
    </section>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="field"><span className="field-label">{label}</span>{children}</label>;
}

function TextField({ label, value, onChange, placeholder, type = 'text' }: { label: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string }) {
  return <Field label={label}><input className="input" type={type} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} /></Field>;
}

function NumberField({ label, value, min, max, onChange }: { label: string; value: number; min?: number; max?: number; onChange: (v: number) => void }) {
  return <Field label={label}><input type="number" className="input" min={min} max={max} value={value} onChange={(e) => onChange(Number(e.target.value) || 0)} /></Field>;
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}

function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-md border border-border bg-surface-2 p-3">
      <div className="text-small font-medium text-text-primary">{label}</div>
      <button type="button" role="switch" aria-checked={checked} onClick={() => onChange(!checked)} className={'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition ' + (checked ? 'bg-klein-500' : 'bg-surface-3')}>
        <span className={'inline-block h-5 w-5 rounded-full bg-white shadow transition transform ' + (checked ? 'translate-x-5' : 'translate-x-0.5')} />
      </button>
    </div>
  );
}
