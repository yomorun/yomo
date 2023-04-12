const colors = require("tailwindcss/colors");

module.exports = {
  content: [
    "./components/**/*.{js, tsx}",
    "./pages/**/*.{md,mdx}",
    "./theme.config.js",
  ],

  theme: {
    extend: {
      fontFamily: {
        sans: [`"Exo 2"`, "sans-serif"],
        mono: [
          "Menlo",
          "Monaco",
          "Lucida Console",
          "Liberation Mono",
          "DejaVu Sans Mono",
          "Bitstream Vera Sans Mono",
          "Courier New",
          "monospace",
        ],
      },
      colors: {
        dark: "#000",
        gray: colors.neutral,
        blue: colors.blue,
        orange: colors.orange,
        green: colors.green,
        red: colors.red,
        yellow: colors.yellow,
      },
      fill: {
        gray: colors.zinc[400],
        white: colors.zinc[100],
        dark: colors.zinc[600],
      },
      screens: {
        sm: "640px",
        md: "768px",
        lg: "1024px",
        betterhover: { raw: "(hover: hover)" },
      },
    },
  },
  variants: {
    extend: {
      display: ['dark']
    },
  },
  darkMode: "class",
};
