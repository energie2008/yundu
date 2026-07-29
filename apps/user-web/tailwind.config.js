/* 调色板劫持：把残留的旧 zinc/indigo/emerald 等硬编码类
   统一映射到暖色主题 RGB 三元组变量（定义在 src/index.css，明暗双套），
   支持 /50 等透明度修饰符，与 admin-web 方案一致 */
const v = (name) => `rgb(var(${name}) / <alpha-value>)`

/* 中性系：旧暗色设计里 100-500 是浅色文字、800-950 是深色底/边框 */
const neutralScale = {
  50: v('--yd-bg'), 100: v('--yd-fg'), 200: v('--yd-fg'), 300: v('--yd-fg-secondary'),
  400: v('--yd-fg-muted'), 500: v('--yd-fg-muted'), 600: v('--yd-fg-muted'),
  700: v('--yd-border-strong'), 800: v('--yd-muted'), 900: v('--yd-card'), 950: v('--yd-bg'),
}

/* 语义系：400-700 主色，200-300 深色可读文字，50-100/800-950 软底 */
const scaleOf = (main, deep, tint) => ({
  50: v(tint), 100: v(tint), 200: v(deep), 300: v(deep),
  400: v(main), 500: v(main), 600: v(main), 700: v(main),
  800: v(tint), 900: v(tint), 950: v(tint),
})
const primaryScale = scaleOf('--yd-primary', '--yd-primary-deep', '--yd-primary-tint')
const successScale = scaleOf('--yd-success', '--yd-success-deep', '--yd-success-tint')
const warningScale = scaleOf('--yd-warning', '--yd-warning-deep', '--yd-warning-tint')
const dangerScale = scaleOf('--yd-danger', '--yd-danger-deep', '--yd-danger-tint')

/** @type {import('tailwindcss').Config} */
const config = {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
    '../../packages/ui/src/**/*.{js,ts,jsx,tsx}',
  ],
  darkMode: 'class',
  theme: {
    extend: {
      /* 语义色板 — 映射到 index.css 的 CSS 变量 */
      colors: {
        background: 'var(--background)',
        foreground: 'var(--foreground)',
        card: {
          DEFAULT: 'var(--card)',
          foreground: 'var(--card-foreground)',
          hover: 'var(--card-hover)',
        },
        primary: {
          DEFAULT: 'var(--primary)',
          soft: 'var(--primary-soft)',
          foreground: 'var(--primary-foreground)',
        },
        secondary: {
          DEFAULT: 'var(--secondary)',
          foreground: 'var(--secondary-foreground)',
        },
        muted: {
          DEFAULT: 'var(--muted)',
          foreground: 'var(--muted-foreground)',
        },
        accent: {
          DEFAULT: 'var(--accent)',
          foreground: 'var(--accent-foreground)',
        },
        destructive: {
          DEFAULT: 'var(--destructive)',
          foreground: 'var(--destructive-foreground)',
        },
        border: 'var(--border)',
        input: 'var(--input)',
        ring: 'var(--ring)',
        /* —— 旧暗色主题硬编码类的整体接管 —— */
        zinc: neutralScale,
        slate: neutralScale,
        gray: neutralScale,
        neutral: neutralScale,
        stone: neutralScale,
        indigo: primaryScale,
        violet: primaryScale,
        purple: primaryScale,
        fuchsia: primaryScale,
        blue: primaryScale,
        sky: primaryScale,
        cyan: primaryScale,
        emerald: successScale,
        green: successScale,
        teal: successScale,
        lime: successScale,
        amber: warningScale,
        yellow: warningScale,
        orange: warningScale,
        red: dangerScale,
        rose: dangerScale,
        pink: dangerScale,
      },
      screens: {
        'mobile': '320px',
        'tablet': '768px',
        'desktop': '1024px',
      },
      keyframes: {
        slideIn: {
          from: { transform: 'translate(-50%, -100%)', opacity: '0' },
          to: { transform: 'translate(-50%, 0)', opacity: '1' },
        },
      },
      animation: {
        slideIn: 'slideIn 0.3s ease-out',
      },
    },
  },
  plugins: [],
}

export default config
