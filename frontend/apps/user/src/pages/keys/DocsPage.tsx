import type { ReactNode } from 'react';

import { Copy } from 'lucide-react';

import { toast } from '../../stores/toast';

const OPENAI_BASE = (import.meta.env.VITE_OPENAI_BASE_URL as string | undefined) ?? '/v1';

const EXAMPLE_BASE =
  typeof window !== 'undefined' && window.location?.origin
    ? `${window.location.origin.replace(/\/$/, '')}/v1`
    : 'https://www.gpt2api.com/v1';

const TEXT_CHAT_SAMPLE = String.raw`curl ${EXAMPLE_BASE}/chat/completions \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-4.20-fast",
    "messages": [
      { "role": "system", "content": "你是一个中文助手" },
      { "role": "user", "content": "帮我写一句产品广告语" }
    ],
    "temperature": 0.7,
    "max_tokens": 512,
    "stream": false
  }'`;

const IMAGE_CREATE_SAMPLE = String.raw`curl ${EXAMPLE_BASE}/images/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "A minimalist product advertisement with a fried chicken bucket placed on a clean white podium.",
    "n": 1,
    "size": "1024x1024",
    "quality": "standard",
    "style": "vivid",
    "async": false
  }'`;

const IMAGE_EDIT_SAMPLE = String.raw`curl ${EXAMPLE_BASE}/images/edits \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "保持主体不变，把背景改成清爽的浅色工作室风格。",
    "image": "https://example.com/input.png",
    "n": 1,
    "size": "1536x1024",
    "quality": "high",
    "async": false
  }'`;

const IMAGE_TASK_SAMPLE = String.raw`curl ${EXAMPLE_BASE}/images/generations/01HTABCDE123456789 \
  -H "Authorization: Bearer sk-xxx"`;

const VIDEO_CREATE_SAMPLE = String.raw`curl ${EXAMPLE_BASE}/video/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-imagine-video",
    "prompt": "一位年轻女生在城市街头自然行走，电影感，稳定镜头",
    "duration": 6,
    "ratio": "16:9",
    "quality": "hd",
    "async": true
  }'`;

const VIDEO_I2V_SAMPLE = String.raw`curl ${EXAMPLE_BASE}/video/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-imagine-video",
    "prompt": "保持参考图人物和构图，做一个轻微镜头推进的短视频。",
    "duration": 10,
    "ratio": "9:16",
    "image": "https://example.com/ref.jpg",
    "async": true
  }'`;

const VIDEO_TASK_SAMPLE = String.raw`curl ${EXAMPLE_BASE}/video/generations/01HTVIDEO123456789 \
  -H "Authorization: Bearer sk-xxx"`;

const PYTHON_SAMPLE = String.raw`from openai import OpenAI

client = OpenAI(
    api_key="sk-xxx",
    base_url="${EXAMPLE_BASE}",
)

# 文字
resp = client.chat.completions.create(
    model="grok-4.20-fast",
    messages=[
        {"role": "system", "content": "你是一个中文助手"},
        {"role": "user", "content": "写一句产品广告语"},
    ],
)
print(resp.choices[0].message.content)

# 生图
img = client.images.generate(
    model="gpt-image-2",
    prompt="A minimalist product advertisement with a fried chicken bucket placed on a clean white podium.",
    n=1,
    size="1024x1024",
)
print(img.data[0].url)

# 也可以直接访问返回的 task_id 再轮询
`;

const ENDPOINTS = [
  {
    method: 'GET',
    path: '/v1/models',
    kind: '模型列表',
    sync: '同步',
    note: '返回当前可用模型。列表由后端动态生成，可能随后台配置变化。',
  },
  {
    method: 'POST',
    path: '/v1/chat/completions',
    kind: '文字对话',
    sync: '同步 / stream',
    note: '支持标准 OpenAI chat 参数；stream=true 时走 SSE 流式返回。',
  },
  {
    method: 'POST',
    path: '/v1/images/generations',
    kind: '图片生成',
    sync: '默认同步，可 async=true',
    note: '支持文生图、图生图、多图输入；异步时返回 task_id，可轮询结果。',
  },
  {
    method: 'POST',
    path: '/v1/images/edits',
    kind: '图片编辑',
    sync: '默认同步，可 async=true',
    note: '必须至少提供一张参考图；适合局部修改、背景替换、风格重绘。',
  },
  {
    method: 'GET',
    path: '/v1/images/generations/:task_id',
    kind: '图片任务',
    sync: '轮询',
    note: '查询图片任务状态，返回任务信息和最终图片链接。',
  },
  {
    method: 'POST',
    path: '/v1/video/generations',
    kind: '视频生成',
    sync: '默认异步，可 async=false',
    note: '支持文生视频、图生视频、多图生视频；默认先返回 task_id，再轮询结果。',
  },
  {
    method: 'GET',
    path: '/v1/video/generations/:task_id',
    kind: '视频任务',
    sync: '轮询',
    note: '查询视频任务状态，返回封面、时长和最终视频链接。',
  },
  {
    method: 'POST',
    path: '/v1/videos/generations',
    kind: '兼容别名',
    sync: '同上',
    note: '旧客户端可继续使用，等价于 /v1/video/generations。',
  },
  {
    method: 'GET',
    path: '/v1/videos/generations/:task_id',
    kind: '兼容别名',
    sync: '同上',
    note: '旧客户端可继续使用，等价于 /v1/video/generations/:task_id。',
  },
] as const;

const IMAGE_SIZES = [
  {
    tier: '1K',
    note: '适合草图、快速出图、成本最低。',
    rows: [
      ['1:1', '1024x1024'],
      ['3:2', '1216x832'],
      ['2:3', '832x1216'],
      ['4:3', '1152x864'],
      ['3:4', '864x1152'],
      ['5:4', '1120x896'],
      ['4:5', '896x1120'],
      ['16:9', '1344x768'],
      ['9:16', '768x1344'],
      ['21:9', '1536x640'],
    ],
  },
  {
    tier: '2K',
    note: '适合常规交付，细节更稳定。',
    rows: [
      ['1:1', '1248x1248'],
      ['3:2', '1536x1024'],
      ['2:3', '1024x1536'],
      ['4:3', '1440x1088'],
      ['3:4', '1088x1440'],
      ['5:4', '1392x1120'],
      ['4:5', '1120x1392'],
      ['16:9', '1664x928'],
      ['9:16', '928x1664'],
      ['21:9', '1904x816'],
    ],
  },
  {
    tier: '4K',
    note: '适合大图、海报、印刷级交付。',
    rows: [
      ['1:1', '2480x2480'],
      ['3:2', '3056x2032'],
      ['2:3', '2032x3056'],
      ['4:3', '2880x2160'],
      ['3:4', '2160x2880'],
      ['5:4', '2784x2224'],
      ['4:5', '2224x2784'],
      ['16:9', '3312x1872'],
      ['9:16', '1872x3312'],
      ['21:9', '3808x1632'],
    ],
  },
] as const;

const VIDEO_NOTES = [
  ['时长', '建议使用 6 秒或 10 秒；接口层允许 2-60 秒，但上游实际限制可能更严格。'],
  ['比例', '推荐 16:9、9:16、1:1；也可通过 ratio / aspect_ratio 传入。'],
  ['参考图', '支持 image、images[]、ref_assets[]；图生视频和多图生视频都走同一接口。'],
  ['异步', '视频默认异步返回 task_id，任务完成后再轮询 GET /v1/video/generations/:task_id。'],
] as const;

export default function DocsPage() {
  const copy = async (s: string, label: string) => {
    await navigator.clipboard.writeText(s);
    toast.success(`${label}已复制`);
  };

  return (
    <div className="page space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">接口文档</h1>
          <p className="page-subtitle leading-loose">
            这里是 <span className="gradient-text">OpenAI 兼容下游接口</span> 的说明页。把官方 SDK 的{' '}
            <code className="kbd mx-1">base_url</code> 切到这里，就能直接接入文字、图片、视频。
          </p>
        </div>
      </header>

      <div className="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
        <DocSection title="接入地址" actionLabel="复制" onCopy={() => copy(OPENAI_BASE, '接入地址')}>
          <div className="rounded-md border border-border bg-surface-2 p-4 font-mono text-body break-all">
            {OPENAI_BASE}
          </div>
          <p className="mt-3 text-small leading-loose text-text-tertiary">
            鉴权方式：<code className="kbd mx-1">Authorization: Bearer sk-xxx</code>。
            API Key 在「Key 管理」里创建。<code className="kbd mx-1">Idempotency-Key</code> 建议在图片/视频请求里带上，避免重复扣费。
          </p>
        </DocSection>

        <DocSection title="调用要点" actionLabel="复制示例" onCopy={() => copy(TEXT_CHAT_SAMPLE, '文字示例')}>
          <ul className="space-y-2 text-sm leading-7 text-text-secondary">
            <li>• 文字接口默认同步返回；stream=true 时走流式输出。</li>
            <li>• 图片接口默认同步等待，async=true 时只返回 task_id。</li>
            <li>• 视频接口默认异步，async=false 时最多等待 10 分钟。</li>
            <li>• 图片编辑必须带参考图；视频图生视频也支持参考图。</li>
          </ul>
        </DocSection>
      </div>

      <DocSection title="接口总览" actionLabel="复制路径" onCopy={() => copy(ENDPOINTS.map((item) => `${item.method} ${item.path}`).join('\n'), '接口路径')}>
        <div className="overflow-x-auto">
          <table className="min-w-full border-separate border-spacing-0 text-sm">
            <thead>
              <tr className="text-left text-text-tertiary">
                <th className="border-b border-border px-3 py-2 font-normal">方法</th>
                <th className="border-b border-border px-3 py-2 font-normal">路径</th>
                <th className="border-b border-border px-3 py-2 font-normal">类型</th>
                <th className="border-b border-border px-3 py-2 font-normal">同步/异步</th>
                <th className="border-b border-border px-3 py-2 font-normal">说明</th>
              </tr>
            </thead>
            <tbody>
              {ENDPOINTS.map((item) => (
                <tr key={`${item.method}-${item.path}`} className="align-top">
                  <td className="border-b border-border px-3 py-3 font-mono text-klein-600">{item.method}</td>
                  <td className="border-b border-border px-3 py-3 font-mono">{item.path}</td>
                  <td className="border-b border-border px-3 py-3">{item.kind}</td>
                  <td className="border-b border-border px-3 py-3 text-text-secondary">{item.sync}</td>
                  <td className="border-b border-border px-3 py-3 text-text-tertiary">{item.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </DocSection>

      <div className="grid gap-4 xl:grid-cols-2">
        <DocSection title="文字 /v1/chat/completions" actionLabel="复制 cURL" onCopy={() => copy(TEXT_CHAT_SAMPLE, '文字 cURL')}>
          <p className="mb-3 text-sm leading-7 text-text-secondary">
            标准 OpenAI Chat 接口。<code className="kbd mx-1">stream=true</code> 时返回 SSE；不传则一次性返回 JSON。
          </p>
          <CodeBlock>{TEXT_CHAT_SAMPLE}</CodeBlock>
          <div className="mt-3 rounded-md border border-border bg-surface-2 p-3 text-small text-text-tertiary leading-7">
            常用参数：<code className="kbd">model</code>、<code className="kbd">messages</code>、<code className="kbd">temperature</code>、<code className="kbd">max_tokens</code>、<code className="kbd">stream</code>。
          </div>
        </DocSection>

        <DocSection title="图片 /v1/images/generations" actionLabel="复制 cURL" onCopy={() => copy(IMAGE_CREATE_SAMPLE, '图片 cURL')}>
          <p className="mb-3 text-sm leading-7 text-text-secondary">
            支持文生图、图生图、多图输入。推荐通过 <code className="kbd mx-1">size</code> 和 <code className="kbd mx-1">quality</code> 控制画质。
          </p>
          <CodeBlock>{IMAGE_CREATE_SAMPLE}</CodeBlock>
          <div className="mt-3 rounded-md border border-border bg-surface-2 p-3 text-small text-text-tertiary leading-7">
            主要参数：<code className="kbd">model</code>、<code className="kbd">prompt</code>、<code className="kbd">n</code>、<code className="kbd">size</code>、<code className="kbd">quality</code>、<code className="kbd">style</code>、<code className="kbd">image/images/ref_assets</code>、<code className="kbd">async</code>。
          </div>
        </DocSection>

        <DocSection title="图片编辑 /v1/images/edits" actionLabel="复制 cURL" onCopy={() => copy(IMAGE_EDIT_SAMPLE, '图片编辑 cURL')}>
          <p className="mb-3 text-sm leading-7 text-text-secondary">
            编辑接口和生成接口共用同一套任务流程，但必须提供至少一张参考图。适合换背景、改风格、局部重绘。
          </p>
          <CodeBlock>{IMAGE_EDIT_SAMPLE}</CodeBlock>
          <div className="mt-3 rounded-md border border-border bg-surface-2 p-3 text-small text-text-tertiary leading-7">
            参考图可以传 <code className="kbd">image</code>、<code className="kbd">images[]</code> 或 <code className="kbd">ref_assets[]</code>；当前不建议直接传本地文件上传。
          </div>
        </DocSection>

        <DocSection title="视频 /v1/video/generations" actionLabel="复制 cURL" onCopy={() => copy(VIDEO_CREATE_SAMPLE, '视频 cURL')}>
          <p className="mb-3 text-sm leading-7 text-text-secondary">
            视频默认异步返回。图生视频时把参考图传到 <code className="kbd mx-1">image</code> / <code className="kbd mx-1">images[]</code> / <code className="kbd mx-1">ref_assets[]</code> 即可。
          </p>
          <CodeBlock>{VIDEO_CREATE_SAMPLE}</CodeBlock>
          <p className="mt-3 mb-3 text-sm leading-7 text-text-secondary">图生视频示例：</p>
          <CodeBlock>{VIDEO_I2V_SAMPLE}</CodeBlock>
          <div className="mt-3 rounded-md border border-border bg-surface-2 p-3 text-small text-text-tertiary leading-7">
            常用参数：<code className="kbd">model</code>、<code className="kbd">prompt</code>、<code className="kbd">duration</code>、<code className="kbd">ratio</code> / <code className="kbd">aspect_ratio</code>、<code className="kbd">quality</code>、<code className="kbd">fps</code>、<code className="kbd">async</code>。
          </div>
        </DocSection>

        <DocSection title="任务轮询" actionLabel="复制示例" onCopy={() => copy(`${IMAGE_TASK_SAMPLE}\n\n${VIDEO_TASK_SAMPLE}`, '任务轮询')}>
          <p className="mb-3 text-sm leading-7 text-text-secondary">
            图片和视频都支持通过 <code className="kbd mx-1">task_id</code> 查询结果。图片接口默认等结果，视频接口默认先返回任务。
          </p>
          <CodeBlock>{`${IMAGE_TASK_SAMPLE}\n\n${VIDEO_TASK_SAMPLE}`}</CodeBlock>
        </DocSection>
      </div>

      <DocSection title="尺寸映射" actionLabel="复制表格" onCopy={() => copy(IMAGE_SIZES.map((tier) => `${tier.tier}\n${tier.rows.map(([ratio, size]) => `${ratio} -> ${size}`).join('\n')}`).join('\n\n'), '尺寸映射')}>
        <div className="space-y-4">
          {IMAGE_SIZES.map((tier) => (
            <div key={tier.tier} className="rounded-xl border border-border bg-surface-1 p-4">
              <div className="mb-3 flex items-center justify-between gap-3">
                <h3 className="section-title">{tier.tier}</h3>
                <span className="text-small text-text-tertiary">{tier.note}</span>
              </div>
              <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
                {tier.rows.map(([ratio, size]) => (
                  <div key={`${tier.tier}-${ratio}`} className="flex items-center justify-between rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm">
                    <span className="text-text-secondary">{ratio}</span>
                    <span className="font-mono text-klein-600">{size}</span>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </DocSection>

      <DocSection title="视频参数补充" actionLabel="复制要点" onCopy={() => copy(VIDEO_NOTES.map(([k, v]) => `${k}: ${v}`).join('\n'), '视频要点')}>
        <div className="grid gap-3 md:grid-cols-2">
          {VIDEO_NOTES.map(([k, v]) => (
            <div key={k} className="rounded-lg border border-border bg-surface-2 p-4">
              <div className="mb-1 text-sm font-medium text-text-primary">{k}</div>
              <div className="text-small leading-7 text-text-tertiary">{v}</div>
            </div>
          ))}
        </div>
      </DocSection>

      <DocSection title="Python 示例" actionLabel="复制代码" onCopy={() => copy(PYTHON_SAMPLE, 'Python 示例')}>
        <CodeBlock>{PYTHON_SAMPLE}</CodeBlock>
      </DocSection>
    </div>
  );
}

function DocSection({
  title,
  actionLabel,
  onCopy,
  children,
}: {
  title: string;
  actionLabel: string;
  onCopy: () => void;
  children: ReactNode;
}) {
  return (
    <section className="card card-section">
      <header className="section-header mb-3">
        <h3 className="section-title">{title}</h3>
        <button className="btn btn-outline btn-sm" onClick={onCopy} type="button">
          <Copy size={14} /> {actionLabel}
        </button>
      </header>
      {children}
    </section>
  );
}

function CodeBlock({ children }: { children: ReactNode }) {
  return (
    <pre className="overflow-x-auto rounded-md border border-border bg-surface-2 p-4 font-mono text-small leading-7">
      {children}
    </pre>
  );
}
