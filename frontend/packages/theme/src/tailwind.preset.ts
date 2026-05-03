/**
 * gpt2api · Tailwind v3 preset
 *
 * 使用方式（apps/* tailwind.config.ts）：
 *   import kleinPreset from '@kleinai/theme/preset';
 *   export default { presets: [kleinPreset], content: [...] };
 */
import type { Config } from 'tailwindcss';

const preset: Partial<Config> = {
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    container: {
      center: true,
      padding: {
        DEFAULT: '1rem',
        sm: '1.25rem',
        lg: '2rem',
        xl: '2.5rem',
        '2xl': '3rem',
      },
      screens: {
        sm: '640px',
        md: '768px',
        lg: '1024px',
        xl: '1280px',
        '2xl': '1536px',
      },
    },
    screens: {
      xs: '480px',
      sm: '640px',
      md: '768px',
      lg: '1024px',
      xl: '1280px',
      '2xl': '1536px',
      '3xl': '1920px',
      '4xl': '2560px',
    },
    extend: {
      fontFamily: {
        sans: 'var(--font-sans)',
        mono: 'var(--font-mono)',
      },
      colors: {
        klein: {
          50: 'var(--klein-50)',
          100: 'var(--klein-100)',
          200: 'var(--klein-200)',
          300: 'var(--klein-300)',
          400: 'var(--klein-400)',
          500: 'var(--klein-500)',
          600: 'var(--klein-600)',
          700: 'var(--klein-700)',
          800: 'var(--klein-800)',
          900: 'var(--klein-900)',
          DEFAULT: 'var(--klein-600)',
        },
        ink: {
          50: 'var(--ink-50)',
          100: 'var(--ink-100)',
          200: 'var(--ink-200)',
          300: 'var(--ink-300)',
          400: 'var(--ink-400)',
          500: 'var(--ink-500)',
          600: 'var(--ink-600)',
          700: 'var(--ink-700)',
          800: 'var(--ink-800)',
          900: 'var(--ink-900)',
        },
        success: { DEFAULT: 'var(--success-500)', soft: 'var(--success-soft)' },
        warning: { DEFAULT: 'var(--warning-500)', soft: 'var(--warning-soft)' },
        danger:  { DEFAULT: 'var(--danger-500)',  soft: 'var(--danger-soft)' },
        info:    { DEFAULT: 'var(--info-500)',    soft: 'var(--info-soft)' },
        surface: {
          bg: 'var(--surface-bg)',
          1: 'var(--surface-1)',
          2: 'var(--surface-2)',
          3: 'var(--surface-3)',
          overlay: 'var(--surface-overlay)',
          glass: 'var(--surface-glass)',
        },
        text: {
          primary: 'var(--text-primary)',
          secondary: 'var(--text-secondary)',
          tertiary: 'var(--text-tertiary)',
          disabled: 'var(--text-disabled)',
          'on-klein': 'var(--text-on-klein)',
        },
        border: {
          DEFAULT: 'var(--border-default)',
          strong: 'var(--border-strong)',
          subtle: 'var(--border-subtle)',
        },
      },
      borderColor: {
        DEFAULT: 'var(--border-default)',
      },
      borderRadius: {
        xs: 'var(--radius-xs)',
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        '2xl': 'var(--radius-2xl)',
        pill: 'var(--radius-pill)',
      },
      boxShadow: {
        1: 'var(--shadow-1)',
        2: 'var(--shadow-2)',
        3: 'var(--shadow-3)',
        4: 'var(--shadow-4)',
        inset: 'var(--shadow-inset)',
        glow: 'var(--klein-glow)',
        'glow-soft': 'var(--klein-glow-soft)',
        'focus-ring': 'var(--focus-ring)',
      },
      backgroundImage: {
        'klein-gradient': 'var(--klein-gradient)',
        'klein-gradient-soft': 'var(--klein-gradient-soft)',
      },
      fontSize: {
        display: ['var(--fs-display)', { lineHeight: 'var(--lh-tight)' }],
        h1:      ['var(--fs-h1)',      { lineHeight: 'var(--lh-tight)' }],
        h2:      ['var(--fs-h2)',      { lineHeight: 'var(--lh-snug)' }],
        h3:      ['var(--fs-h3)',      { lineHeight: 'var(--lh-snug)' }],
        h4:      ['var(--fs-h4)',      { lineHeight: 'var(--lh-snug)' }],
        body:    ['var(--fs-body)',    { lineHeight: 'var(--lh-normal)' }],
        small:   ['var(--fs-small)',   { lineHeight: 'var(--lh-normal)' }],
        tiny:    ['var(--fs-tiny)',    { lineHeight: 'var(--lh-snug)' }],
      },
      fontWeight: {
        regular:  'var(--weight-regular)',
        medium:   'var(--weight-medium)',
        semibold: 'var(--weight-semibold)',
        bold:     'var(--weight-bold)',
        extra:    'var(--weight-extra)',
      },
      letterSpacing: {
        tighter: 'var(--tracking-tight)',
        normal:  'var(--tracking-normal)',
        wide:    'var(--tracking-wide)',
        wider:   'var(--tracking-wider)',
      },
      spacing: {
        '0.5': 'var(--space-0_5)',
        '1':   'var(--space-1)',
        '1.5': 'var(--space-1_5)',
        '2':   'var(--space-2)',
        '2.5': 'var(--space-2_5)',
        '3':   'var(--space-3)',
        '3.5': 'var(--space-3_5)',
        '4':   'var(--space-4)',
        '5':   'var(--space-5)',
        '6':   'var(--space-6)',
        '7':   'var(--space-7)',
        '8':   'var(--space-8)',
        '10':  'var(--space-10)',
        '12':  'var(--space-12)',
        '16':  'var(--space-16)',
        '20':  'var(--space-20)',
      },
      transitionTimingFunction: {
        klein: 'var(--ease-in-out)',
        'klein-out': 'var(--ease-out)',
        'klein-spring': 'var(--ease-spring)',
      },
      transitionDuration: {
        fast: 'var(--duration-fast)',
        base: 'var(--duration-base)',
        slow: 'var(--duration-slow)',
      },
      keyframes: {
        'klein-fade-in': {
          from: { opacity: '0', transform: 'translateY(8px)' },
          to:   { opacity: '1', transform: 'translateY(0)' },
        },
        'klein-shimmer': {
          '0%':   { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' },
        },
      },
      animation: {
        'klein-fade-in': 'klein-fade-in .25s cubic-bezier(.4,0,.2,1) both',
        'klein-shimmer': 'klein-shimmer 1.6s linear infinite',
      },
    },
  },
};

export default preset;
