/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // A near-black ground with a warm accent — the app is used at 6am and
        // at 11pm, so the palette is built dark-first.
        ink: {
          950: '#08080a',
          900: '#0e0e11',
          850: '#141419',
          800: '#1b1b22',
          700: '#26262f',
          600: '#3a3a46',
          500: '#5a5a6b',
          400: '#8b8b9c',
          300: '#b8b8c6',
          200: '#d9d9e2',
          100: '#eeeef3',
        },
        flame: {
          500: '#ff6b35',
          400: '#ff8659',
          600: '#e8541f',
        },
        moss: {
          500: '#37d67a',
          400: '#5ee094',
          600: '#25b862',
        },
      },
      fontFamily: {
        sans: ['-apple-system', 'BlinkMacSystemFont', 'Inter', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      keyframes: {
        // The check-off press: a quick overshoot then settle. This is the
        // animation the whole app is built around.
        pop: {
          '0%': { transform: 'scale(1)' },
          '40%': { transform: 'scale(1.14)' },
          '70%': { transform: 'scale(0.97)' },
          '100%': { transform: 'scale(1)' },
        },
        ripple: {
          '0%': { transform: 'scale(0.6)', opacity: '0.55' },
          '100%': { transform: 'scale(2.4)', opacity: '0' },
        },
        'slide-up': {
          '0%': { transform: 'translateY(12px)', opacity: '0' },
          '100%': { transform: 'translateY(0)', opacity: '1' },
        },
        'fade-in': {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
      },
      animation: {
        pop: 'pop 420ms cubic-bezier(0.34, 1.56, 0.64, 1)',
        ripple: 'ripple 600ms ease-out forwards',
        'slide-up': 'slide-up 280ms cubic-bezier(0.16, 1, 0.3, 1) both',
        'fade-in': 'fade-in 200ms ease-out both',
      },
    },
  },
  plugins: [],
}
