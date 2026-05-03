import { Copy } from 'lucide-react';

import { toast } from '../../stores/toast';

const OPENAI_BASE = (import.meta.env.VITE_OPENAI_BASE_URL as string | undefined) ?? '/v1';

const RESOLVED_BASE = OPENAI_BASE.startsWith('http')
  ? OPENAI_BASE
  : 'https://api.gpt2api.example/v1';

const PY_SAMPLE = `from openai import OpenAI

client = OpenAI(
    api_key="sk-xxx",
    base_url="${RESOLVED_BASE}",
)

resp = client.images.generate(
    model="img-v3",
    prompt="a futuristic floating castle at sunset, volumetric light",
    n=2,
    size="1024x1024",
)
print(resp.data[0].url)`;

const CURL_SAMPLE = `curl ${RESOLVED_BASE}/videos/generations \\
  -H "Authorization: Bearer sk-xxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "vid-v1",
    "prompt": "a neon-lit alley scene, slow-mo, cinematic",
    "duration": 8,
    "size": "16:9"
  }'`;

export default function DocsPage() {
  const copy = (s: string, label: string) =>
    navigator.clipboard.writeText(s).then(() => toast.success(`${label}已复制`));

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1 className="page-title">调用说明</h1>
          <p className="page-subtitle leading-loose">
            gpt2api 提供 <span className="gradient-text">OpenAI 兼容协议</span>，把官方 SDK 的
            <code className="kbd mx-1">base_url</code>切成下面这一行就能用。
          </p>
        </div>
      </header>

      <DocSection
        title="基础地址"
        actionLabel="复制"
        onCopy={() => copy(OPENAI_BASE, '基础地址')}
      >
        <div className="font-mono text-body p-4 rounded-md bg-surface-2 text-klein-500 break-all">
          {OPENAI_BASE}
        </div>
        <p className="mt-3 text-small text-text-tertiary leading-loose">
          鉴权请使用「KEY 管理」页创建的 <code className="kbd">sk-…</code> 明文，放入
          <code className="kbd mx-1">Authorization: Bearer …</code> 头即可。
        </p>
      </DocSection>

      <DocSection
        title="Python · 生图示例"
        actionLabel="复制代码"
        onCopy={() => copy(PY_SAMPLE, '示例代码')}
      >
        <CodeBlock>{PY_SAMPLE}</CodeBlock>
      </DocSection>

      <DocSection
        title="cURL · 生视频示例"
        actionLabel="复制 cURL"
        onCopy={() => copy(CURL_SAMPLE, 'cURL')}
      >
        <CodeBlock>{CURL_SAMPLE}</CodeBlock>
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
  children: React.ReactNode;
}) {
  return (
    <section className="card card-section mb-4">
      <header className="section-header mb-3">
        <h3 className="section-title">{title}</h3>
        <button className="btn btn-outline btn-sm" onClick={onCopy}>
          <Copy size={14} /> {actionLabel}
        </button>
      </header>
      {children}
    </section>
  );
}

function CodeBlock({ children }: { children: React.ReactNode }) {
  return (
    <pre className="font-mono text-small p-4 rounded-md bg-surface-2 border border-border overflow-x-auto leading-loose">
      {children}
    </pre>
  );
}
