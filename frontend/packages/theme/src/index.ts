export { default as kleinPreset } from './tailwind.preset';

export const KLEIN_TOKENS = {
  primary: 'var(--klein-600)',
  primaryGradient: 'var(--klein-gradient)',
  glow: 'var(--klein-glow)',
} as const;

export type ThemeMode = 'light' | 'dark' | 'system';

export function applyThemeMode(mode: ThemeMode) {
  const root = document.documentElement;
  if (mode === 'system') {
    const dark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    root.dataset.theme = dark ? 'dark' : 'light';
    return;
  }
  root.dataset.theme = mode;
}
