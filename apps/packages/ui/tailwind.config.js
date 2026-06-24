/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: ['class'],
  theme: {
    container: {
      center: true,
      padding: {
        DEFAULT: '1rem',
        sm: '1.5rem',
        lg: '2rem',
      },
      screens: {
        '2xl': '1400px',
      },
    },
    extend: {
      fontFamily: {
        sans: ['var(--font-inter)', 'system-ui', 'sans-serif'],
        serif: ['var(--font-fraunces)', 'Georgia', 'serif'],
      },
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
      },
      colors: {
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        card: {
          DEFAULT: 'hsl(var(--card))',
          foreground: 'hsl(var(--card-foreground))',
        },
        popover: {
          DEFAULT: 'hsl(var(--popover))',
          foreground: 'hsl(var(--popover-foreground))',
        },
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))',
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary))',
          foreground: 'hsl(var(--secondary-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        chart: {
          1: 'hsl(var(--chart-1))',
          2: 'hsl(var(--chart-2))',
          3: 'hsl(var(--chart-3))',
          4: 'hsl(var(--chart-4))',
          5: 'hsl(var(--chart-5))',
        },
        // Brand color palette
        brand: {
          'light-green': 'hsl(var(--brand-light-green))',
          green: 'hsl(var(--brand-green))',
          'summer-sky': 'hsl(var(--brand-summer-sky))',
          'deep-teal': 'hsl(var(--brand-deep-teal))',
        },
        grey: {
          'light-1': 'hsl(var(--grey-light-1))',
          'light-2': 'hsl(var(--grey-light-2))',
          'light-3': 'hsl(var(--grey-light-3))',
          'dark-1': 'hsl(var(--grey-dark-1))',
          'dark-2': 'hsl(var(--grey-dark-2))',
          'dark-3': 'hsl(var(--grey-dark-3))',
        },
      },
    },
  },
  plugins: [require('tailwindcss-animate')],
};
