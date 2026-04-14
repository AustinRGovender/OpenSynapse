/**
 * OpenSynapse design tokens.
 *
 * All colours in the product come from this file.
 * No hex colours should appear anywhere else in component source.
 * See docs/05-ui-ux-spec.md for the full specification.
 */

export const colors = {
  // Neutrals (slate scale)
  slate: {
    50: '#f8fafc',
    100: '#f1f5f9',
    200: '#e2e8f0',
    300: '#cbd5e1',
    400: '#94a3b8',
    500: '#64748b',
    600: '#475569',
    700: '#334155',
    800: '#1e293b',
    900: '#0f172a',
    950: '#020617',
  },

  // Accent (teal)
  teal: {
    50: '#f0fdfa',
    300: '#5eead4',
    400: '#2dd4bf',
    500: '#0d9488',
    600: '#0f766e',
    700: '#115e59',
  },

  // Semantic
  success: '#10b981',
  warning: '#f59e0b',
  error: '#ef4444',
  info: '#3b82f6',

  // Semantic backgrounds (50-level tints)
  successBg: '#ecfdf5',
  warningBg: '#fffbeb',
  errorBg: '#fef2f2',
  infoBg: '#eff6ff',

  // Chart palette (accessible, colour-blind friendly)
  chart: {
    1: '#0d9488',
    2: '#8b5cf6',
    3: '#f59e0b',
    4: '#14b8a6',
    5: '#ec4899',
    6: '#6366f1',
    7: '#84cc16',
    8: '#06b6d4',
  },
} as const

export const typography = {
  fontFamily: {
    ui: 'Inter, -apple-system, system-ui, sans-serif',
    code: 'JetBrains Mono, Menlo, Consolas, monospace',
  },
  fontSize: {
    xs: '11px',
    sm: '13px',
    base: '14px',
    lg: '16px',
    xl: '20px',
    '2xl': '24px',
    '3xl': '30px',
  },
  fontWeight: {
    regular: 400,
    medium: 500,
    semibold: 600,
    bold: 700,
  },
  lineHeight: {
    body: 1.5,
    heading: 1.2,
  },
} as const

export const spacing = {
  0: '0px',
  1: '4px',
  2: '8px',
  3: '12px',
  4: '16px',
  5: '20px',
  6: '24px',
  8: '32px',
  10: '40px',
  12: '48px',
  16: '64px',
} as const

export const motion = {
  duration: {
    fast: '150ms',
    normal: '250ms',
    slow: '400ms',
  },
  easing: 'cubic-bezier(0.2, 0, 0, 1)',
} as const

export const radii = {
  sm: '4px',
  md: '6px',
  lg: '8px',
  xl: '12px',
} as const
