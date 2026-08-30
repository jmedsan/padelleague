/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["../views/**/*.html"],
  plugins: [require("daisyui"), require("@tailwindcss/typography")],
  daisyui: {
    themes: [
      "nord",
      {
        dark: {
          "primary": "#6C9BCF",
          "primary-content": "#FFFFFF",
          "secondary": "#7B8CDE",
          "secondary-content": "#FFFFFF",
          "accent": "#80CBC4",
          "accent-content": "#1D232A",
          "neutral": "#2A303C",
          "neutral-content": "#C6D0DC",
          "base-100": "#1D232A",
          "base-200": "#262D35",
          "base-300": "#2F3640",
          "base-content": "#C6D0DC",
          "info": "#66C7F4",
          "info-content": "#1D232A",
          "success": "#6BC49E",
          "success-content": "#1D232A",
          "warning": "#E5B960",
          "warning-content": "#1D232A",
          "error": "#E57373",
          "error-content": "#1D232A",
        },
      },
    ],
  },
}
