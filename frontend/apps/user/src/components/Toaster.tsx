import clsx from 'clsx';
import { CheckCircle2, X, AlertTriangle, Info } from 'lucide-react';

import { useToastStore } from '../stores/toast';

const ICONS = {
  success: CheckCircle2,
  error: AlertTriangle,
  info: Info,
} as const;

const COLOR = {
  success: 'border-success bg-surface-1 text-success',
  error: 'border-danger bg-surface-1 text-danger',
  info: 'border-klein-500 bg-surface-1 text-klein-500',
} as const;

export function Toaster() {
  const items = useToastStore((s) => s.items);
  const dismiss = useToastStore((s) => s.dismiss);
  return (
    <div className="fixed top-4 right-4 z-[100] flex flex-col gap-2 max-w-[min(92vw,360px)]">
      {items.map((t) => {
        const Icon = ICONS[t.kind];
        return (
          <div
            key={t.id}
            className={clsx(
              'flex items-start gap-3 px-4 py-3 rounded-md border shadow-3 backdrop-blur',
              'bg-surface-1/95 text-text-primary',
              'klein-fade-in',
              COLOR[t.kind],
            )}
          >
            <Icon size={18} className="shrink-0 mt-0.5" />
            <p className="flex-1 text-small text-text-primary leading-loose break-all">{t.msg}</p>
            <button
              aria-label="关闭"
              className="text-text-tertiary hover:text-text-primary transition"
              onClick={() => dismiss(t.id)}
            >
              <X size={16} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
