import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
    '../../packages/ui/src/**/*.{js,ts,jsx,tsx}',
  ],
  darkMode: 'class',
  theme: {
    extend: {
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
