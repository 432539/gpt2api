// 后台展示格式化工具。

const numberFmt = new Intl.NumberFormat('zh-CN');

/** 后端 points（*100） → 展示数值 */
export function fmtPoints(p: number | undefined | null): string {
  if (p == null) return '0';
  return numberFmt.format(p / 100);
}

export function fmtNumber(n: number | undefined | null): string {
  if (n == null) return '0';
  return numberFmt.format(n);
}

export function fmtTime(ts?: number): string {
  if (!ts) return '—';
  const d = new Date(ts * 1000);
  if (Number.isNaN(d.getTime())) return '—';
  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fmtRelative(ts?: number): string {
  if (!ts) return '—';
  const diff = Date.now() / 1000 - ts;
  if (diff < 60) return `${Math.max(0, Math.floor(diff))} 秒前`;
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} 天前`;
  return fmtTime(ts);
}

export function statusLabel(s: number): { label: string; tone: 'ok' | 'warn' | 'err' | 'mute' } {
  switch (s) {
    case 1:
      return { label: '正常', tone: 'ok' };
    case 0:
      return { label: '禁用', tone: 'mute' };
    case 2:
      return { label: '熔断', tone: 'warn' };
    case -1:
      return { label: '已删除', tone: 'err' };
    default:
      return { label: String(s), tone: 'mute' };
  }
}
