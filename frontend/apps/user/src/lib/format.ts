// 格式化工具。所有点数后端均按 *100 存储。

const numberFmt = new Intl.NumberFormat('zh-CN', {
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
});

/** 后端 points（*100） → 展示数值 */
export function fmtPoints(p: number | undefined | null): string {
  if (p == null) return '0';
  const v = p / 100;
  return numberFmt.format(v);
}

/** 后端 unix 秒 → 本地化时间字符串 */
export function fmtTime(ts: number | undefined | null): string {
  if (!ts) return '—';
  const d = new Date(ts * 1000);
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** 后端 unix 秒 → 相对时间（仅粗略） */
export function fmtRelative(ts: number | undefined | null): string {
  if (!ts) return '—';
  const diff = Date.now() / 1000 - ts;
  if (diff < 60) return '刚刚';
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  if (diff < 86400 * 7) return `${Math.floor(diff / 86400)} 天前`;
  return fmtTime(ts);
}

const BIZ_LABEL: Record<string, string> = {
  recharge: '充值',
  cdk: 'CDK 兑换',
  promo: '优惠码',
  invite: '邀请奖励',
  refund: '失败退款',
  consume: '消费',
  freeze: '冻结',
  unfreeze: '解冻',
  grant: '系统赠点',
};

export function fmtBiz(biz: string): string {
  return BIZ_LABEL[biz] ?? biz;
}

export function pointsClass(direction: number): string {
  return direction > 0 ? 'text-success' : 'text-danger';
}
