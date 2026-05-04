import clsx from 'clsx';

interface LogoProps {
  size?: 'sm' | 'md' | 'lg';
  iconOnly?: boolean;
  suffix?: string;
  className?: string;
}

const SIZE: Record<NonNullable<LogoProps['size']>, { icon: number; text: string }> = {
  sm: { icon: 24, text: 'text-small' },
  md: { icon: 30, text: 'text-h4' },
  lg: { icon: 40, text: 'text-h3' },
};

export function Logo({ size = 'md', iconOnly = false, suffix, className }: LogoProps) {
  const cfg = SIZE[size];

  return (
    <div className={clsx('flex min-w-0 select-none items-center gap-2', className)}>
      <span
        className="grid shrink-0 place-items-center rounded-xl bg-neutral-950 font-semibold text-white"
        style={{ width: cfg.icon, height: cfg.icon }}
      >
        AI
      </span>
      {!iconOnly && (
        <span className={clsx(cfg.text, 'leading-none font-medium tracking-tight text-text-primary')}>
          Studio
          {suffix && <span className="ml-2 align-middle text-tiny font-medium text-text-tertiary">{suffix}</span>}
        </span>
      )}
    </div>
  );
}
