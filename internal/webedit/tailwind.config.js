import colors from 'tailwindcss/colors';

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./**/*.{html,templ}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        accent: {
          light: colors.blue[600],
          dark: colors.blue[400],
        },
      },
    },
  },
  plugins: [], // Tailwind plugins only
};
