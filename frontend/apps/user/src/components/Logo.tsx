import clsx from 'clsx';

const LOGO_SRC = '/logo-icon.png?v=20260501';

interface LogoProps {
  size?: 'sm' | 'md' | 'lg';
  /** 仅图标，不渲染文字（移动端紧凑场景） */
  iconOnly?: boolean;
  /** 文字附加后缀，例如「管理后台」 */
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
    <div className={clsx('flex items-center gap-2 select-none min-w-0', className)}>
      <img
        src={LOGO_SRC}
        alt="首页"
        height={cfg.icon}
        style={{ height: cfg.icon, width: 'auto' }}
        draggable={false}
        className="block object-contain shrink-0"
      />
      {!iconOnly && (
        <span className={clsx(cfg.text, 'font-medium tracking-tight text-text-primary leading-none')}>
          首页
          {suffix && <span className="ml-2 align-middle text-tiny font-medium text-text-tertiary">{suffix}</span>}
        </span>
      )}
    </div>
  );
}
